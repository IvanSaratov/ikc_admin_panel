package runner

import (
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
)

func TestResolveServeConfig(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	resolved, err := ResolveServeConfig(ServeConfig{
		Runtime:           RuntimeConfig{Environment: "dev"},
		Address:           ":8080",
		DatabasePath:      filepath.Join("data", "ikc.db"),
		BootstrapPassword: "bootstrap-secret",
		Frontend:          "disabled",
		SessionTTL:        8 * time.Hour,
		CookieSameSite:    "lax",
		CSRFKey:           "csrf-key",
		TrustedOrigins:    " first.example, ,second.example ",
		PlaintextCSRF:     true,
	})
	if err != nil {
		t.Fatalf("ResolveServeConfig: %v", err)
	}
	if want := filepath.Join(root, "data", "ikc.db"); resolved.DatabasePath != want {
		t.Fatalf("database path = %q, want %q", resolved.DatabasePath, want)
	}
	if resolved.Address != ":8080" || resolved.BootstrapPassword != "bootstrap-secret" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if resolved.Session.TTL != 8*time.Hour || resolved.Session.SameSite != http.SameSiteLaxMode || resolved.Session.Secure {
		t.Fatalf("session = %#v", resolved.Session)
	}
	if resolved.CSRF.Key != "csrf-key" || !resolved.CSRF.Plaintext {
		t.Fatalf("csrf = %#v", resolved.CSRF)
	}
	if want := []string{"first.example", "second.example"}; !reflect.DeepEqual(resolved.CSRF.TrustedOrigins, want) {
		t.Fatalf("trusted origins = %#v, want %#v", resolved.CSRF.TrustedOrigins, want)
	}
	if resolved.Frontend.Mode != app.FrontendDisabled {
		t.Fatalf("frontend = %#v", resolved.Frontend)
	}
}

func TestResolveServeConfigDerivesSecureCookieInProduction(t *testing.T) {
	resolved, err := ResolveServeConfig(ServeConfig{
		Runtime: RuntimeConfig{Environment: "production"}, DatabasePath: filepath.Join(t.TempDir(), "ikc.db"),
		Frontend: "embedded", SessionTTL: time.Hour, CookieSameSite: "lax",
	})
	if err != nil || !resolved.Session.Secure {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	resolved, err = ResolveServeConfig(ServeConfig{
		Runtime: RuntimeConfig{Environment: "production"}, DatabasePath: filepath.Join(t.TempDir(), "ikc.db"),
		Frontend: "embedded", SessionTTL: time.Hour, CookieSameSite: "lax",
		CookieSecure: OptionalBool{Set: true, Value: false},
	})
	if err != nil || resolved.Session.Secure {
		t.Fatalf("explicit false ignored: %#v %v", resolved, err)
	}
}

func TestResolveServeConfigRejectsInvalidFrontend(t *testing.T) {
	for _, frontend := range []string{"", "external", "EMBEDDED"} {
		t.Run(frontend, func(t *testing.T) {
			_, err := ResolveServeConfig(ServeConfig{
				DatabasePath: filepath.Join(t.TempDir(), "ikc.db"),
				Frontend:     frontend, SessionTTL: time.Hour, CookieSameSite: "lax",
			})
			if err == nil {
				t.Fatalf("frontend %q accepted", frontend)
			}
		})
	}
}

func TestResolveDatabaseConfigResolvesAbsoluteCleanPath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	resolved, err := ResolveDatabaseConfig(DatabaseConfig{
		Runtime:      RuntimeConfig{Environment: "dev"},
		DatabasePath: filepath.Join("data", "..", "database", "ikc.db"),
	})
	if err != nil {
		t.Fatalf("ResolveDatabaseConfig: %v", err)
	}
	if want := filepath.Join(root, "database", "ikc.db"); resolved.DatabasePath != want {
		t.Fatalf("database path = %q, want %q", resolved.DatabasePath, want)
	}
}

func TestResolveDatabaseConfigRedactsRejectedPath(t *testing.T) {
	secretPath := filepath.Join("customer-secret", "database.db")
	for _, environment := range []string{"prod", "production", "PROD", " production "} {
		t.Run(environment, func(t *testing.T) {
			_, err := ResolveDatabaseConfig(DatabaseConfig{
				Runtime: RuntimeConfig{Environment: environment}, DatabasePath: secretPath,
			})
			if err == nil || strings.Contains(err.Error(), secretPath) {
				t.Fatalf("error = %v", err)
			}
			if !strings.Contains(err.Error(), "IKC_SERVER_DB") {
				t.Fatalf("error does not name IKC_SERVER_DB: %v", err)
			}
		})
	}
}

func TestDatabaseActions(t *testing.T) {
	want := []DatabaseAction{"status", "migrate", "verify", "backup"}
	got := []DatabaseAction{DatabaseStatus, DatabaseMigrate, DatabaseVerify, DatabaseBackup}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("database actions = %#v, want %#v", got, want)
	}
}
