package legacy

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewLogrusAdapterWithLoggerUsesProvidedLogger(t *testing.T) {
	var out bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&out)
	logger.SetFormatter(&logrus.JSONFormatter{})

	adapter := NewLogrusAdapterWithLogger(logger)
	adapter.Warn("legacy-docx-warning")

	if !strings.Contains(out.String(), "legacy-docx-warning") {
		t.Fatalf("log output missing legacy warning: %s", out.String())
	}
}
