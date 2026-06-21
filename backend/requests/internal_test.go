package requests

// Internal-package tests for security-sensitive helpers in handler.go.
// Lives in `package requests` (not `requests_test`) so it can reach
// unexported identifiers like sha256Hex.

import (
	"strings"
	"testing"
)

func TestSha256Hex_NoControlChars(t *testing.T) {
	t.Parallel()

	// Malicious input mimicking a log-injection attempt: carriage
	// returns, newlines, NUL bytes, ANSI escape sequences.
	malicious := "evil\r\nFAKE LOG LINE\x00\x1b[31mRED\x1b[0m"
	sum := sha256Hex([]byte(malicious))

	if strings.ContainsAny(sum, "\r\n\x00") {
		t.Errorf("sha256Hex output contains control chars: %q", sum)
	}
	if len(sum) != 64 {
		t.Errorf("sha256Hex length = %d, want 64 hex chars", len(sum))
	}

	// Same fingerprint for same input — deterministic.
	again := sha256Hex([]byte(malicious))
	if sum != again {
		t.Errorf("sha256Hex not deterministic: %q vs %q", sum, again)
	}

	// Different input → different fingerprint.
	other := sha256Hex([]byte("different"))
	if sum == other {
		t.Errorf("sha256Hex collision for distinct inputs")
	}
}
