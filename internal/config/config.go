package config

import (
	"flag"
	"os"
	"strconv"
)

type Config struct {
	Host    string
	Port    int
	DBPath  string
	DataDir string
}

func Load() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.Host, "host", envOrDefault("CYRENE_HOST", "0.0.0.0"), "Host address to bind")
	flag.IntVar(&cfg.Port, "port", envIntOrDefault("CYRENE_PORT", 20128), "Port to bind the gateway")
	flag.StringVar(&cfg.DBPath, "db", envOrDefault("CYRENE_DB", "data.sqlite"), "Path to SQLite database")
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
