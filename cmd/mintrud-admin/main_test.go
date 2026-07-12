package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.uber.org/zap/zapcore"
)

// TestEnv verifies the env() helper returns the env var when set and
// the fallback otherwise. Used by run() to read MINTRUD_ADMIN_ADDR
// and MINTRUD_ADMIN_DB.
func TestEnv_ReturnsValueWhenSet(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_TEST_KEY", "value-from-env")
	if got := env("MINTRUD_ADMIN_TEST_KEY", "fallback"); got != "value-from-env" {
		t.Errorf("env() = %q, want value-from-env", got)
	}
}

func TestEnv_ReturnsFallbackWhenUnset(t *testing.T) {
	// t.Setenv with empty string IS set, so use os.Unsetenv via t.Setenv
	// (t.Setenv automatically restores on cleanup).
	t.Setenv("MINTRUD_ADMIN_TEST_KEY", "")
	if got := env("MINTRUD_ADMIN_TEST_KEY", "fallback"); got != "fallback" {
		t.Errorf("env() = %q, want fallback", got)
	}
}

func TestNewLoggerFromEnv_UsesRuntimeEnv(t *testing.T) {
	t.Setenv("MINTRUD_ADMIN_ENV", "prod")
	t.Setenv("MINTRUD_ADMIN_LOG_LEVEL", "debug")
	t.Setenv("MINTRUD_ADMIN_LOG_FORMAT", "json")

	var out bytes.Buffer
	logger, err := newLoggerFromEnv(&out)
	if err != nil {
		t.Fatalf("newLoggerFromEnv: %v", err)
	}
	if !logger.Core().Enabled(zapcore.DebugLevel) {
		t.Fatalf("debug level disabled, want enabled")
	}
	logger.Debug("debug env log")
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &entry); err != nil {
		t.Fatalf("parse json log %q: %v", out.String(), err)
	}
	if entry["level"] != "debug" {
		t.Fatalf("level = %v, want debug", entry["level"])
	}
}
