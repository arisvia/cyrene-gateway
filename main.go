package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
)

//go:embed templates/*
var templateFS embed.FS

func main() {
	port := flag.Int("port", 20128, "Port to bind the gateway")
	dbPath := flag.String("db", "data.sqlite", "Path to SQLite database")
	flag.Parse()

	// Using Go 1.21 Structured Logging (slog)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("Starting cyrene-gateway", slog.Int("port", *port), slog.String("db", *dbPath))

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		slog.Warn("Database file not found. System will fallback to memory adapters.", slog.String("path", *dbPath))
	}

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"service":"cyrene-gateway","status":"active"}`))
	})

	addr := fmt.Sprintf(":%d", *port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("Gateway terminated with critical error", "error", err)
		os.Exit(1)
	}
}
