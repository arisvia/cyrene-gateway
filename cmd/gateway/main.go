package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/handler"
)

func main() {
	cfg := config.Load()

	// Structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("Starting cyrene-gateway",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("dataDir", cfg.DataDir),
	)

	// Ensure data directory exists, then initialize database
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		slog.Error("Failed to create data directory", "error", err)
		os.Exit(1)
	}

	// Initialize the auth signing secret explicitly after config load. It
	// lives in the fixed application data directory and is separated from
	// password hashing salts (P1-6).
	if err := auth.InitSecretManager(cfg.DataDir, cfg.Secret); err != nil {
		slog.Error("Failed to initialize auth secret", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Create HTTP server
	srv := handler.NewServer(database, cfg)

	// Attempt to download latest panel (non-blocking, falls back to embedded)
	go srv.Dashboard.TryDownload()

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      srv.Handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Gateway listening", slog.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server terminated with critical error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("Shutting down gracefully...")

	// Stop MITM proxy (removes DNS entries) if it was running
	srv.MITM.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Forced shutdown", "error", err)
	}

	slog.Info("Gateway stopped")
}
