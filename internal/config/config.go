package config

import (
	"flag"
	"os"
)

type Config struct {
	Port    int
	DBPath  string
	DataDir string
}

func Load() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.Port, "port", 20128, "Port to bind the gateway")
	flag.StringVar(&cfg.DBPath, "db", "data.sqlite", "Path to SQLite database")
	flag.Parse()

	cfg.DataDir = os.Getenv("DATA_DIR")
	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DataDir = home + "/.cyrene-gateway"
	}

	return cfg
}
