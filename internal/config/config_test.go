package config

import (
	"flag"
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

func TestFlagContract(t *testing.T) {
	cfg := &Config{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	registerFlags(fs, cfg)

	args := []string{
		"-host", "127.0.0.1",
		"--port", "30000",
		"-data-dir", "/tmp/cyrene-data",
		"--dashboard", "/tmp/cyrene-ui",
		"-panel-url", "https://example.com/panel.zip",
		"--secret", "custom-hmac-secret-32-chars-long",
		"-allow-private-networks",
		"-v",
	}

	if err := fs.Parse(args); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.Port != 30000 {
		t.Errorf("expected port 30000, got %d", cfg.Port)
	}
	if cfg.DataDir != "/tmp/cyrene-data" {
		t.Errorf("expected data-dir /tmp/cyrene-data, got %s", cfg.DataDir)
	}
	if cfg.Dashboard != "/tmp/cyrene-ui" {
		t.Errorf("expected dashboard /tmp/cyrene-ui, got %s", cfg.Dashboard)
	}
	if cfg.PanelURL != "https://example.com/panel.zip" {
		t.Errorf("expected panel-url, got %s", cfg.PanelURL)
	}
	if cfg.Secret != "custom-hmac-secret-32-chars-long" {
		t.Errorf("expected secret, got %s", cfg.Secret)
	}
	if !cfg.AllowPrivateNetworks {
		t.Errorf("expected allow-private-networks true")
	}
	if !cfg.ShowVersion {
		t.Errorf("expected show-version true")
	}
}
