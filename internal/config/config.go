package config

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Host                 string
	Port                 int
	DBPath               string
	DataDir              string
	Dashboard            string
	PanelURL             string
	Secret               string
	MITM                 bool
	MITMPort             int
	AllowPrivateNetworks bool
}

func Load() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.DataDir, "data-dir", envOrDefault("CYRENE_DATA_DIR", ""), "Data directory for database, panel cache and secrets (default ~/.cyrene-gateway)")
	flag.IntVar(&cfg.Port, "port", envIntOrDefault("CYRENE_PORT", 20128), "Port to bind the gateway")
	flag.StringVar(&cfg.Dashboard, "dashboard", envOrDefault("CYRENE_DASHBOARD", ""), "Local dashboard directory path (empty=use embedded)")
	flag.StringVar(&cfg.PanelURL, "panel-url", envOrDefault("CYRENE_PANEL_URL", ""), "URL to download updated panel (dist.zip auto-extracted, or single HTML; empty=use embedded)")
	flag.StringVar(&cfg.Secret, "secret", envOrDefault("CYRENE_SECRET", ""), "Dashboard access password")
	flag.BoolVar(&cfg.MITM, "mitm", false, "Enable MITM proxy (local deployments only, requires localhost bind)")
	flag.IntVar(&cfg.MITMPort, "mitm-port", envIntOrDefault("CYRENE_MITM_PORT", 443), "MITM proxy listen port")
	flag.BoolVar(&cfg.AllowPrivateNetworks, "allow-private-networks", envOrDefault("CYRENE_ALLOW_PRIVATE_NETWORKS", "") == "true" || envOrDefault("CYRENE_ALLOW_PRIVATE_NETWORKS", "") == "1", "Allow upstream proxying to private/loopback IP addresses (for local testing/mock servers)")
	flag.Parse()

	home, _ := os.UserHomeDir()
	defaultDir := filepath.Join(home, ".cyrene-gateway")
	if cfg.DataDir == "" {
		cfg.DataDir = defaultDir
	}
	cfg.DBPath = filepath.Join(cfg.DataDir, "data.sqlite")

	return cfg
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
