package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestNewDefaultsToInfoText(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Output: &out})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger.Core().Enabled(zapcore.DebugLevel) {
		t.Fatalf("debug level enabled, want disabled by default")
	}
	if !logger.Core().Enabled(zapcore.InfoLevel) {
		t.Fatalf("info level disabled, want enabled by default")
	}
	logger.Info("hello", zapcore.Field{Key: "component", Type: zapcore.StringType, String: "test"})
	if got := out.String(); !bytes.Contains([]byte(got), []byte("hello")) || !bytes.Contains([]byte(got), []byte("component")) {
		t.Fatalf("text log output = %q, want message and field", got)
	}
}

func TestNewUsesJSONInProd(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Env: "prod", Output: &out})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("prod log")
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &entry); err != nil {
		t.Fatalf("parse json log %q: %v", out.String(), err)
	}
	if entry["msg"] != "prod log" {
		t.Fatalf("msg = %v, want prod log", entry["msg"])
	}
}

func TestNewAcceptsExplicitLevelAndFormat(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Level: "debug", Format: "json", Output: &out})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !logger.Core().Enabled(zapcore.DebugLevel) {
		t.Fatalf("debug level disabled, want enabled")
	}
	logger.Debug("debug log")
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &entry); err != nil {
		t.Fatalf("parse json log %q: %v", out.String(), err)
	}
	if entry["level"] != "debug" {
		t.Fatalf("level = %v, want debug", entry["level"])
	}
}

func TestNewRejectsInvalidLevel(t *testing.T) {
	if _, err := New(Config{Level: "verbose"}); err == nil {
		t.Fatalf("New invalid level: got nil error")
	}
}

func TestNewRejectsInvalidFormat(t *testing.T) {
	if _, err := New(Config{Format: "xml"}); err == nil {
		t.Fatalf("New invalid format: got nil error")
	}
}
