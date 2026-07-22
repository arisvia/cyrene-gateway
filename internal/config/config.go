package config

import (
	"flag"
	"os"
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
}

func Load() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Host, "host", envOrDefault("CYRENE_HOST", "0.0.0.0"), "Host address to bind")
	flag.IntVar(&cfg.Port, "port", envIntOrDefault("CYRENE_PORT", 20128), "Port to bind the gateway")
	flag.StringVar(&cfg.DBPath, "db", envOrDefault("CYRENE_DB", "data.sqlite"), "Path to SQLite database")
	flag.StringVar(&cfg.Dashboard, "dashboard", envOrDefault("CYRENE_DASHBOARD", ""), "Local dashboard directory path (empty=use embedded)")
	flag.StringVar(&cfg.PanelURL, "panel-url", envOrDefault("CYRENE_PANEL_URL", "https://raw.githubusercontent.com/arisvia/cyrene-gateway/main/templates/index.html"), "URL to download updated panel")
	flag.StringVar(&cfg.Secret, "secret", envOrDefault("CYRENE_SECRET", ""), "Dashboard access password")
	flag.Parse()

	cfg.DataDir = os.Getenv("DATA_DIR")
	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DataDir = home + "/.cyrene-gateway"
	}

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
