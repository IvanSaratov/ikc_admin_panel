package main

import "testing"

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
