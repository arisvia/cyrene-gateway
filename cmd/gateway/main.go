package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/handler"
)

func main() {
	cfg := config.Load()

	// Auth secret: -secret flag wins; else load/generate <data-dir>/auth-secret
	auth.SetSecret(cfg.Secret)
	auth.InitSecretFile(cfg.DataDir)

	// Structured logging with web SSE broadcast
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler.NewBroadcastLogHandler(jsonHandler))
	slog.SetDefault(logger)
	slog.Info("Starting cyrene-gateway",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("dataDir", cfg.DataDir),
	)

	// Ensure data directory exists, then initialize database
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("Failed to create data directory", "error", err)
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

	// Start background model discovery & health probing ticker (every 6 hours)
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	srv.StartBackgroundModelSync(bgCtx, 6*time.Hour)

	httpServer := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           srv.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	serverErr := make(chan error, 1)

	go func() {
		slog.Info("Gateway listening", slog.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		slog.Error("Server terminated with critical error", "error", err)
		return
	case <-done:
		slog.Info("Shutting down gracefully...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Forced shutdown", "error", err)
	}

	slog.Info("Gateway stopped")
}
