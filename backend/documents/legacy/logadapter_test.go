package legacy

import (
	"bytes"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewZapAdapterWithLoggerUsesProvidedLogger(t *testing.T) {
	var out bytes.Buffer
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&out),
		zapcore.DebugLevel,
	))

	adapter := NewZapAdapterWithLogger(logger)
	adapter.Warn("legacy-docx-warning")

	if !strings.Contains(out.String(), "legacy-docx-warning") {
		t.Fatalf("log output missing legacy warning: %s", out.String())
	}
}
