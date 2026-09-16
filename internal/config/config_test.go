package config

import (
	"os"
	"testing"
)

func TestEnvOrDefault(t *testing.T) {
	const key = "CYRENE_TEST_ENV_STRING"
	os.Unsetenv(key)
	if got := envOrDefault(key, "fallback"); got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}

	os.Setenv(key, "custom")
	defer os.Unsetenv(key)
	if got := envOrDefault(key, "fallback"); got != "custom" {
		t.Errorf("expected 'custom', got %q", got)
	}
}

func TestEnvIntOrDefault(t *testing.T) {
	const key = "CYRENE_TEST_ENV_INT"
	os.Unsetenv(key)
	if got := envIntOrDefault(key, 8080); got != 8080 {
		t.Errorf("expected 8080, got %d", got)
	}

	os.Setenv(key, "9090")
	defer os.Unsetenv(key)
	if got := envIntOrDefault(key, 8080); got != 9090 {
		t.Errorf("expected 9090, got %d", got)
	}

	os.Setenv(key, "invalid")
	if got := envIntOrDefault(key, 8080); got != 8080 {
		t.Errorf("expected 8080 for invalid int, got %d", got)
	}
}
