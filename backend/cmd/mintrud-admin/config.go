package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type runtimeConfig struct {
	Addr   string
	DBPath string
}

func loadRuntimeConfig() (runtimeConfig, error) {
	rawPath := env("MINTRUD_ADMIN_DB", filepath.Join("data", "mintrud-admin.db"))
	if env("MINTRUD_ADMIN_ENV", "dev") == "prod" && !filepath.IsAbs(rawPath) {
		return runtimeConfig{}, fmt.Errorf("MINTRUD_ADMIN_DB must be absolute in prod")
	}

	absolutePath, err := filepath.Abs(rawPath)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("resolve database path: %w", err)
	}

	return runtimeConfig{
		Addr:   env("MINTRUD_ADMIN_ADDR", ":8080"),
		DBPath: filepath.Clean(absolutePath),
	}, nil
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
