package admin

// Internal-package tests for security-sensitive helpers and sentinels.
// Lives in `package admin` (not `admin_test`) so it can see unexported
// identifiers like isSafeRedirect and ErrUserDisabled.

import (
	"errors"
	"testing"
)

func TestIsSafeRedirect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"empty rejected", "", false},
		{"plain relative path", "/programs", true},
		{"path with query", "/programs?x=1", true},
		{"path with fragment", "/programs#section", true},
		{"double slash rejected", "//evil.com", false},
		{"triple slash accepted as path", "///evil.com", true},
		{"backslash rejected", `/\\evil.com`, false},
		{"backslash anywhere rejected", "/path\\evil", false},
		{"percent-encoded slash rejected", "/path%2fevil", false},
		{"percent-encoded backslash rejected", "/path%5cevil", false},
		{"absolute http rejected", "http://evil.com", false},
		{"absolute https rejected", "https://evil.com", false},
		{"scheme only rejected", "javascript:alert(1)", false},
		{"relative non-path rejected", "evil", false},
		{"relative with leading dot rejected", "./evil", false},
		{"relative with parent rejected", "../evil", false},
		{"host-bearing url rejected", "//evil.com/path", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isSafeRedirect(tc.target); got != tc.want {
				t.Errorf("isSafeRedirect(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestErrUserDisabledIsErrInvalidCredentials(t *testing.T) {
	t.Parallel()
	// Security: the disabled-account branch must collapse to the same
	// sentinel as wrong-password so callers cannot distinguish them.
	if ErrUserDisabled != ErrInvalidCredentials {
		t.Fatalf("ErrUserDisabled must alias ErrInvalidCredentials; got %v vs %v", ErrUserDisabled, ErrInvalidCredentials)
	}
	if !errors.Is(ErrUserDisabled, ErrInvalidCredentials) {
		t.Fatalf("errors.Is(ErrUserDisabled, ErrInvalidCredentials) must be true")
	}
}
