package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/internal/runner"
	"github.com/urfave/cli/v3"
)

type capturedActions struct {
	serve    runner.ServeConfig
	database runner.DatabaseConfig
	action   runner.DatabaseAction
}

func TestCommandRequiresExplicitSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCommand(&stdout, &stderr, commandActions{})
	if err := command.Run(context.Background(), []string{"renamed-client.exe"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(stdout.String(), "renamed-client.exe") || !strings.Contains(stdout.String(), "serve") {
		t.Fatalf("help = %q", stdout.String())
	}
}

func TestServeCLIOverridesEnvironment(t *testing.T) {
	t.Setenv("IKC_SERVER_ADDR", ":9000")
	var captured runner.ServeConfig
	actions := commandActions{serve: func(_ context.Context, cfg runner.ServeConfig, _ io.Writer) error {
		captured = cfg
		return nil
	}}
	command := newCommand(io.Discard, io.Discard, actions)
	err := command.Run(context.Background(), []string{"server", "serve", "--address", ":9100"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if captured.Address != ":9100" {
		t.Fatalf("address = %q", captured.Address)
	}
}

func TestFlagsUseNarrowestScope(t *testing.T) {
	actions := commandActions{
		serve:    func(context.Context, runner.ServeConfig, io.Writer) error { return nil },
		database: func(context.Context, runner.DatabaseAction, runner.DatabaseConfig, io.Writer) error { return nil },
	}
	command := newCommand(io.Discard, io.Discard, actions)
	if err := command.Run(context.Background(), []string{"server", "db", "--address", ":9000", "status"}); err == nil {
		t.Fatal("db accepted serve-only --address")
	}
	if err := command.Run(context.Background(), []string{"server", "serve", "--database", "serve.db"}); err != nil {
		t.Fatalf("serve database flag: %v", err)
	}
}

func findFlag(t *testing.T, command *cli.Command, name string) cli.Flag {
	t.Helper()
	for _, flag := range command.Flags {
		if slices.Contains(flag.Names(), name) {
			return flag
		}
	}
	t.Fatalf("flag %q not found on %q", name, command.Name)
	return nil
}

func TestFlagEnvironmentSourcesAndVisibility(t *testing.T) {
	root := newCommand(io.Discard, io.Discard, commandActions{})
	tests := []struct{ scope, name, environment string }{
		{"root", "environment", "IKC_SERVER_ENV"},
		{"root", "log-level", "IKC_SERVER_LOG_LEVEL"},
		{"root", "log-format", "IKC_SERVER_LOG_FORMAT"},
		{"serve", "address", "IKC_SERVER_ADDR"},
		{"serve", "database", "IKC_SERVER_DB"},
		{"serve", "bootstrap-password", "IKC_SERVER_BOOTSTRAP_PASSWORD"},
		{"serve", "frontend", "IKC_SERVER_FRONTEND"},
		{"serve", "session-ttl", "IKC_SERVER_SESSION_TTL"},
		{"serve", "cookie-secure", "IKC_SERVER_COOKIE_SECURE"},
		{"serve", "cookie-same-site", "IKC_SERVER_COOKIE_SAMESITE"},
		{"serve", "csrf-key", "IKC_SERVER_CSRF_KEY"},
		{"serve", "plaintext-csrf", "IKC_SERVER_PLAINTEXT_CSRF"},
		{"serve", "trusted-origins", "IKC_SERVER_TRUSTED_ORIGINS"},
		{"db", "database", "IKC_SERVER_DB"},
	}
	for _, test := range tests {
		t.Run(test.scope+"/"+test.name, func(t *testing.T) {
			command := root
			if test.scope != "root" {
				command = root.Command(test.scope)
			}
			flag := findFlag(t, command, test.name)
			envFlag, ok := flag.(interface{ GetEnvVars() []string })
			if !ok {
				t.Fatal("flag does not expose environment sources")
			}
			if !slices.Contains(envFlag.GetEnvVars(), test.environment) {
				t.Fatalf("env sources = %#v", envFlag.GetEnvVars())
			}
			visible, ok := flag.(interface{ IsVisible() bool })
			if !ok || !visible.IsVisible() {
				t.Fatal("flag is hidden")
			}
		})
	}
}

func TestDatabaseActions(t *testing.T) {
	for _, action := range []runner.DatabaseAction{
		runner.DatabaseStatus, runner.DatabaseMigrate,
		runner.DatabaseVerify, runner.DatabaseBackup,
	} {
		t.Run(string(action), func(t *testing.T) {
			var captured runner.DatabaseAction
			command := newCommand(io.Discard, io.Discard, commandActions{
				database: func(_ context.Context, got runner.DatabaseAction, _ runner.DatabaseConfig, _ io.Writer) error {
					captured = got
					return nil
				},
			})
			if err := command.Run(context.Background(), []string{"server", "db", string(action)}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if captured != action {
				t.Fatalf("action = %q", captured)
			}
		})
	}
}

func TestServeConfigDefaultsAndOptionalCookieSecure(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_ADDR", ":9999")
	for _, test := range []struct {
		args       []string
		set, value bool
	}{
		{args: []string{"server", "serve"}},
		{args: []string{"server", "serve", "--cookie-secure=true"}, set: true, value: true},
		{args: []string{"server", "serve", "--cookie-secure=false"}, set: true, value: false},
	} {
		var captured runner.ServeConfig
		command := newCommand(io.Discard, io.Discard, commandActions{
			serve: func(_ context.Context, cfg runner.ServeConfig, _ io.Writer) error {
				captured = cfg
				return nil
			},
		})
		if err := command.Run(context.Background(), test.args); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if captured.Address != ":8080" || captured.DatabasePath != filepath.Join("data", "ikc.db") {
			t.Fatalf("defaults = %#v", captured)
		}
		if captured.Runtime.Environment != "dev" || captured.Runtime.LogLevel != "info" ||
			captured.Runtime.LogFormat != "" || captured.Frontend != "embedded" ||
			captured.SessionTTL != 8*time.Hour || captured.CookieSameSite != "lax" ||
			captured.PlaintextCSRF {
			t.Fatalf("defaults = %#v", captured)
		}
		if captured.CookieSecure.Set != test.set || captured.CookieSecure.Value != test.value {
			t.Fatalf("cookie secure = %#v", captured.CookieSecure)
		}
	}
}
