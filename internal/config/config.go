package config

import (
	"flag"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Host      string
	Port      int
	DBPath    string
	DataDir   string
	Dashboard string
	PanelURL  string
	Secret    string
	MITM      bool
	MITMPort  int
}

func Load() *Config {
	cfg := &Config{}

	// Secure by default (37A): bind loopback only. Remote listeners require an
	// explicit -host/CYRENE_HOST override plus configured dashboard login.
	flag.StringVar(&cfg.Host, "host", envOrDefault("CYRENE_HOST", "127.0.0.1"), "Host address to bind")
	flag.IntVar(&cfg.Port, "port", envIntOrDefault("CYRENE_PORT", 20128), "Port to bind the gateway")
	flag.StringVar(&cfg.Dashboard, "dashboard", envOrDefault("CYRENE_DASHBOARD", ""), "Local dashboard directory path (empty=use embedded)")
	flag.StringVar(&cfg.PanelURL, "panel-url", envOrDefault("CYRENE_PANEL_URL", ""), "URL to download updated panel (dist.zip auto-extracted, or single HTML; empty=use embedded)")
	flag.StringVar(&cfg.Secret, "secret", envOrDefault("CYRENE_SECRET", ""), "Explicit session/API-key signing secret (stored in the data dir when empty)")
	flag.BoolVar(&cfg.MITM, "mitm", false, "Enable MITM proxy (local deployments only, requires localhost bind)")
	flag.IntVar(&cfg.MITMPort, "mitm-port", envIntOrDefault("CYRENE_MITM_PORT", 443), "MITM proxy listen port")
	flag.Parse()

	home, _ := os.UserHomeDir()
	cfg.DataDir = filepath.Join(home, ".cyrene-gateway")
	cfg.DBPath = filepath.Join(cfg.DataDir, "data.sqlite")

	return cfg
}

// IsLoopbackBind reports whether the configured host binds only to loopback.
// An empty host binds all interfaces and is NOT loopback.
func (c *Config) IsLoopbackBind() bool {
	if c.Host == "localhost" {
		return true
	}
	ip := net.ParseIP(c.Host)
	return ip != nil && ip.IsLoopback()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
