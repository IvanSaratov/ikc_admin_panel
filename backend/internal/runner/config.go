package runner

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/IvanSaratov/ikc_admin_panel/backend/admin"
	"github.com/IvanSaratov/ikc_admin_panel/backend/app"
)

type RuntimeConfig struct {
	Environment string
	LogLevel    string
	LogFormat   string
}

type OptionalBool struct {
	Value bool
	Set   bool
}

type ServeConfig struct {
	Runtime                     RuntimeConfig
	Address, DatabasePath       string
	BootstrapPassword, Frontend string
	SessionTTL                  time.Duration
	CookieSecure                OptionalBool
	CookieSameSite, CSRFKey     string
	TrustedOrigins              string
	PlaintextCSRF               bool
}

type DatabaseConfig struct {
	Runtime      RuntimeConfig
	DatabasePath string
}

type DatabaseAction string

const (
	DatabaseStatus  DatabaseAction = "status"
	DatabaseMigrate DatabaseAction = "migrate"
	DatabaseVerify  DatabaseAction = "verify"
	DatabaseBackup  DatabaseAction = "backup"
)

type ResolvedServeConfig struct {
	Address, DatabasePath, BootstrapPassword string
	Session                                  admin.SessionConfig
	CSRF                                     admin.CSRFConfig
	Frontend                                 app.FrontendConfig
}

type ResolvedDatabaseConfig struct {
	DatabasePath string
}

func ResolveServeConfig(config ServeConfig) (ResolvedServeConfig, error) {
	databasePath, err := resolveDatabasePath(config.Runtime, config.DatabasePath)
	if err != nil {
		return ResolvedServeConfig{}, err
	}

	secure := isProduction(config.Runtime.Environment)
	if config.CookieSecure.Set {
		secure = config.CookieSecure.Value
	}
	session, err := admin.NewSessionConfig(config.SessionTTL, config.CookieSameSite, secure)
	if err != nil {
		return ResolvedServeConfig{}, fmt.Errorf("resolve session config: %w", err)
	}

	var frontend app.FrontendConfig
	switch config.Frontend {
	case string(app.FrontendEmbedded):
		frontend.Mode = app.FrontendEmbedded
	case string(app.FrontendDisabled):
		frontend.Mode = app.FrontendDisabled
	default:
		return ResolvedServeConfig{}, fmt.Errorf("frontend must be one of embedded|disabled, got %q", config.Frontend)
	}

	return ResolvedServeConfig{
		Address:           config.Address,
		DatabasePath:      databasePath,
		BootstrapPassword: config.BootstrapPassword,
		Session:           session,
		CSRF: admin.CSRFConfig{
			Key:            config.CSRFKey,
			TrustedOrigins: splitCSV(config.TrustedOrigins),
			Plaintext:      config.PlaintextCSRF,
		},
		Frontend: frontend,
	}, nil
}

func ResolveDatabaseConfig(config DatabaseConfig) (ResolvedDatabaseConfig, error) {
	databasePath, err := resolveDatabasePath(config.Runtime, config.DatabasePath)
	if err != nil {
		return ResolvedDatabaseConfig{}, err
	}
	return ResolvedDatabaseConfig{DatabasePath: databasePath}, nil
}

func resolveDatabasePath(runtime RuntimeConfig, databasePath string) (string, error) {
	if isProduction(runtime.Environment) && !filepath.IsAbs(databasePath) {
		return "", fmt.Errorf("IKC_SERVER_DB must be absolute in production")
	}
	absolutePath, err := filepath.Abs(databasePath)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	return filepath.Clean(absolutePath), nil
}

func isProduction(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func splitCSV(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
