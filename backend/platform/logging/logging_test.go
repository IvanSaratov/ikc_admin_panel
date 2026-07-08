package logging

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewDefaultsToInfoText(t *testing.T) {
	var out bytes.Buffer
	logger, err := New(Config{Output: &out})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger.Level != logrus.InfoLevel {
		t.Fatalf("level = %v, want info", logger.Level)
	}
	if _, ok := logger.Formatter.(*logrus.TextFormatter); !ok {
		t.Fatalf("formatter = %T, want TextFormatter", logger.Formatter)
	}
}

func TestNewUsesJSONInProd(t *testing.T) {
	logger, err := New(Config{Env: "prod"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := logger.Formatter.(*logrus.JSONFormatter); !ok {
		t.Fatalf("formatter = %T, want JSONFormatter", logger.Formatter)
	}
}

func TestNewAcceptsExplicitLevelAndFormat(t *testing.T) {
	logger, err := New(Config{Level: "debug", Format: "json"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger.Level != logrus.DebugLevel {
		t.Fatalf("level = %v, want debug", logger.Level)
	}
	if _, ok := logger.Formatter.(*logrus.JSONFormatter); !ok {
		t.Fatalf("formatter = %T, want JSONFormatter", logger.Formatter)
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
