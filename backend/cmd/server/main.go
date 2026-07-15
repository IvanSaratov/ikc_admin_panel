package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/internal/runner"
	"github.com/urfave/cli/v3"
)

type commandActions struct {
	serve    func(context.Context, runner.ServeConfig, io.Writer) error
	database func(context.Context, runner.DatabaseAction, runner.DatabaseConfig, io.Writer) error
}

func main() {
	command := newCommand(os.Stdout, os.Stderr, commandActions{
		serve:    runner.Serve,
		database: runner.RunDatabase,
	})
	if err := command.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func newCommand(stdout, stderr io.Writer, actions commandActions) *cli.Command {
	runtimeConfig := func(cmd *cli.Command) runner.RuntimeConfig {
		return runner.RuntimeConfig{
			Environment: cmd.String("environment"),
			LogLevel:    cmd.String("log-level"),
			LogFormat:   cmd.String("log-format"),
		}
	}
	serveConfig := func(cmd *cli.Command) runner.ServeConfig {
		return runner.ServeConfig{
			Runtime:           runtimeConfig(cmd),
			Address:           cmd.String("address"),
			DatabasePath:      cmd.String("database"),
			BootstrapPassword: cmd.String("bootstrap-password"),
			Frontend:          cmd.String("frontend"),
			SessionTTL:        cmd.Duration("session-ttl"),
			CookieSecure: runner.OptionalBool{
				Value: cmd.Bool("cookie-secure"),
				Set:   cmd.IsSet("cookie-secure"),
			},
			CookieSameSite: cmd.String("cookie-same-site"),
			CSRFKey:        cmd.String("csrf-key"),
			PlaintextCSRF:  cmd.Bool("plaintext-csrf"),
			TrustedOrigins: cmd.String("trusted-origins"),
		}
	}
	databaseConfig := func(cmd *cli.Command) runner.DatabaseConfig {
		return runner.DatabaseConfig{
			Runtime:      runtimeConfig(cmd),
			DatabasePath: cmd.String("database"),
		}
	}
	databaseCommand := func(name string, action runner.DatabaseAction) *cli.Command {
		return &cli.Command{
			Name: name,
			Action: func(ctx context.Context, cmd *cli.Command) error {
				return actions.database(ctx, action, databaseConfig(cmd), stdout)
			},
		}
	}

	return &cli.Command{
		Writer:    stdout,
		ErrWriter: stderr,
		Usage:     "run the IKC expert admin server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "environment",
				Value:   "dev",
				Sources: cli.EnvVars("IKC_SERVER_ENV"),
			},
			&cli.StringFlag{
				Name:    "log-level",
				Value:   "info",
				Sources: cli.EnvVars("IKC_SERVER_LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Sources: cli.EnvVars("IKC_SERVER_LOG_FORMAT"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "start the HTTP server",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "address",
						Value:   ":8080",
						Sources: cli.EnvVars("IKC_SERVER_ADDR"),
					},
					&cli.StringFlag{
						Name:    "database",
						Value:   filepath.Join("data", "ikc.db"),
						Sources: cli.EnvVars("IKC_SERVER_DB"),
					},
					&cli.StringFlag{
						Name:    "bootstrap-password",
						Sources: cli.EnvVars("IKC_SERVER_BOOTSTRAP_PASSWORD"),
					},
					&cli.StringFlag{
						Name:    "frontend",
						Value:   "embedded",
						Sources: cli.EnvVars("IKC_SERVER_FRONTEND"),
					},
					&cli.DurationFlag{
						Name:    "session-ttl",
						Value:   8 * time.Hour,
						Sources: cli.EnvVars("IKC_SERVER_SESSION_TTL"),
					},
					&cli.BoolFlag{
						Name:    "cookie-secure",
						Sources: cli.EnvVars("IKC_SERVER_COOKIE_SECURE"),
					},
					&cli.StringFlag{
						Name:    "cookie-same-site",
						Value:   "lax",
						Sources: cli.EnvVars("IKC_SERVER_COOKIE_SAMESITE"),
					},
					&cli.StringFlag{
						Name:    "csrf-key",
						Sources: cli.EnvVars("IKC_SERVER_CSRF_KEY"),
					},
					&cli.BoolFlag{
						Name:    "plaintext-csrf",
						Sources: cli.EnvVars("IKC_SERVER_PLAINTEXT_CSRF"),
					},
					&cli.StringFlag{
						Name:    "trusted-origins",
						Sources: cli.EnvVars("IKC_SERVER_TRUSTED_ORIGINS"),
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return actions.serve(ctx, serveConfig(cmd), stdout)
				},
			},
			{
				Name:  "db",
				Usage: "inspect and maintain the SQLite database",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "database",
						Value:   filepath.Join("data", "ikc.db"),
						Sources: cli.EnvVars("IKC_SERVER_DB"),
					},
				},
				Commands: []*cli.Command{
					databaseCommand(string(runner.DatabaseStatus), runner.DatabaseStatus),
					databaseCommand(string(runner.DatabaseMigrate), runner.DatabaseMigrate),
					databaseCommand(string(runner.DatabaseVerify), runner.DatabaseVerify),
					databaseCommand(string(runner.DatabaseBackup), runner.DatabaseBackup),
				},
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowRootCommandHelp(cmd)
		},
	}
}
