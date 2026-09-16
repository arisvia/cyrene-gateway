package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Host                 string
	DBPath               string
	DataDir              string
	Dashboard            string
	PanelURL             string
	Secret               string
	Port                 int
	AllowPrivateNetworks bool
	ShowVersion          bool
}

func Load() *Config {
	cfg := &Config{}
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Cyrene Gateway - High-performance LLM API gateway with embedded Solid.js console\n\n")
		fmt.Fprintf(out, "Usage:\n  %s [flags]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(out, "Server Flags:\n")
		fmt.Fprintf(out, "  -host string\n    \tHost/IP to bind the gateway (default %q, env: CYRENE_HOST)\n", envOrDefault("CYRENE_HOST", "0.0.0.0"))
		fmt.Fprintf(out, "  -port int\n    \tPort to bind the gateway (default %d, env: CYRENE_PORT)\n", envIntOrDefault("CYRENE_PORT", 20128))
		fmt.Fprintf(out, "  -data-dir string\n    \tData directory for database, panel cache and secrets (default ~/.cyrene-gateway, env: CYRENE_DATA_DIR)\n\n")
		fmt.Fprintf(out, "Security Flags:\n")
		fmt.Fprintf(out, "  -secret string\n    \tHMAC master secret for signing API keys and session tokens (env: CYRENE_SECRET)\n")
		fmt.Fprintf(out, "  -allow-private-networks\n    \tAllow upstream proxying to private/loopback IP addresses (for local testing/mock servers, env: CYRENE_ALLOW_PRIVATE_NETWORKS)\n\n")
		fmt.Fprintf(out, "Dashboard & UI Flags:\n")
		fmt.Fprintf(out, "  -dashboard string\n    \tLocal directory to serve dashboard from for dev mode (empty=use embedded, env: CYRENE_DASHBOARD)\n")
		fmt.Fprintf(out, "  -panel-url string\n    \tURL to download updated panel (dist.zip auto-extracted, or single HTML; empty=use embedded, env: CYRENE_PANEL_URL)\n\n")
		fmt.Fprintf(out, "General Flags:\n")
		fmt.Fprintf(out, "  -v, -version\n    \tPrint version and exit\n")
		fmt.Fprintf(out, "  -h, -help\n    \tShow this help message\n")
	}

	flag.StringVar(&cfg.Host, "host", envOrDefault("CYRENE_HOST", "0.0.0.0"), "Host/IP to bind the gateway (env: CYRENE_HOST)")
	flag.StringVar(&cfg.DataDir, "data-dir", envOrDefault("CYRENE_DATA_DIR", ""), "Data directory for database, panel cache and secrets (default ~/.cyrene-gateway, env: CYRENE_DATA_DIR)")
	flag.IntVar(&cfg.Port, "port", envIntOrDefault("CYRENE_PORT", 20128), "Port to bind the gateway (env: CYRENE_PORT)")
	flag.StringVar(&cfg.Dashboard, "dashboard", envOrDefault("CYRENE_DASHBOARD", ""), "Local directory to serve dashboard from for dev mode (empty=use embedded, env: CYRENE_DASHBOARD)")
	flag.StringVar(&cfg.PanelURL, "panel-url", envOrDefault("CYRENE_PANEL_URL", ""), "URL to download updated panel (dist.zip auto-extracted, or single HTML; empty=use embedded, env: CYRENE_PANEL_URL)")
	flag.StringVar(&cfg.Secret, "secret", envOrDefault("CYRENE_SECRET", ""), "HMAC master secret for signing API keys and session tokens (env: CYRENE_SECRET)")
	flag.BoolVar(&cfg.AllowPrivateNetworks, "allow-private-networks", envOrDefault("CYRENE_ALLOW_PRIVATE_NETWORKS", "") == "true" || envOrDefault("CYRENE_ALLOW_PRIVATE_NETWORKS", "") == "1", "Allow upstream proxying to private/loopback IP addresses (for local testing/mock servers, env: CYRENE_ALLOW_PRIVATE_NETWORKS)")
	flag.BoolVar(&cfg.ShowVersion, "v", false, "Print version and exit")
	flag.BoolVar(&cfg.ShowVersion, "version", false, "Print version and exit")
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
