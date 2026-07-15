package runner

import (
	"bytes"
	"errors"
	"io"
	"log"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestWithLoggerRedirectsGlobalsAndReturnsActionError(t *testing.T) {
	var output bytes.Buffer
	wantErr := errors.New("synthetic action failure")
	err := withLogger(RuntimeConfig{Environment: "dev", LogLevel: "debug", LogFormat: "text"}, &output,
		func(logger *zap.Logger) error {
			zap.L().Debug("global zap message")
			log.Print("standard log message")
			return wantErr
		})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v", err)
	}
	for _, message := range []string{"global zap message", "standard log message"} {
		if !strings.Contains(output.String(), message) {
			t.Fatalf("missing %q in %q", message, output.String())
		}
	}
}

func TestWithLoggerRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []RuntimeConfig{
		{Environment: "dev", LogLevel: "not-a-level"},
		{Environment: "dev", LogFormat: "not-a-format"},
	} {
		err := withLogger(test, io.Discard, func(*zap.Logger) error { return nil })
		if err == nil {
			t.Fatalf("configuration accepted: %#v", test)
		}
	}
}
