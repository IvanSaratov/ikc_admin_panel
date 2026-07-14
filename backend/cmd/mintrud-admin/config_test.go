package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRuntimeConfig_ResolvesRelativeDatabasePathInDevelopment(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("MINTRUD_ADMIN_ENV", "dev")
	t.Setenv("MINTRUD_ADMIN_ADDR", "")
	t.Setenv("MINTRUD_ADMIN_DB", filepath.Join("data", "app.db"))

	config, err := loadRuntimeConfig()
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}

	wantPath := filepath.Join(root, "data", "app.db")
	if config.DBPath != wantPath {
		t.Errorf("DBPath = %q, want %q", config.DBPath, wantPath)
	}
	if config.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", config.Addr)
	}
}

func TestLoadRuntimeConfig_RejectsRelativeDatabasePathInProductionWithoutLeakingIt(t *testing.T) {
	rawPath := filepath.Join("private", "customer.db")
	t.Setenv("MINTRUD_ADMIN_ENV", "prod")
	t.Setenv("MINTRUD_ADMIN_DB", rawPath)

	_, err := loadRuntimeConfig()
	if err == nil {
		t.Fatal("loadRuntimeConfig error = nil, want relative path rejection")
	}
	if strings.Contains(err.Error(), rawPath) {
		t.Errorf("loadRuntimeConfig error contains configured database path: %q", err)
	}
}

func TestLoadRuntimeConfig_AcceptsAbsoluteDatabasePathInProduction(t *testing.T) {
	rawPath := filepath.Join(t.TempDir(), "database", "app.db")
	t.Setenv("MINTRUD_ADMIN_ENV", "prod")
	t.Setenv("MINTRUD_ADMIN_ADDR", "127.0.0.1:9090")
	t.Setenv("MINTRUD_ADMIN_DB", rawPath)

	config, err := loadRuntimeConfig()
	if err != nil {
		t.Fatalf("loadRuntimeConfig: %v", err)
	}

	if config.DBPath != filepath.Clean(rawPath) {
		t.Errorf("DBPath = %q, want %q", config.DBPath, filepath.Clean(rawPath))
	}
	if config.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q, want 127.0.0.1:9090", config.Addr)
	}
}
