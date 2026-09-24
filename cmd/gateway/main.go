package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/handler"
	"github.com/arisvia/cyrene-gateway/internal/updater"
)

func main() {
	cfg := config.Load()
	if cfg.ShowVersion {
		fmt.Printf("cyrene-gateway %s\n", handler.Version())
		return
	}
	updater.CleanupFailedBinary("")

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

	// Ensure admin password exists or apply override from CLI flag / env var
	ensureAdminPassword(database, cfg.AdminPassword)

	// Create HTTP server
	srv := handler.NewServer(database, cfg)

	// Attempt to download latest panel (non-blocking, falls back to embedded)
	go srv.Dashboard.TryDownload()

	// Start background model discovery & health probing ticker (every 6 hours)
	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	srv.StartBackgroundModelSync(bgCtx, 6*time.Hour)

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	isRestart := os.Getenv("CYRENE_RESTART") == "1"
	maxRetries := 1
	if isRestart {
		maxRetries = 30 // retry for up to 6 seconds during restart port handover
	}

	var ln net.Listener
	for i := range maxRetries {
		var err error
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		if i == maxRetries-1 {
			slog.Error("Failed to bind gateway port", "addr", addr, "error", err)
			os.Exit(1)
		}
		time.Sleep(200 * time.Millisecond)
	}

	httpServer := newHTTPServer(ln.Addr().String(), srv.Handler)
	var serverMu sync.Mutex
	stopping := false
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(done)
	serverErr := make(chan error, 1)
	serve := func(server *http.Server, listener net.Listener) {
		slog.Info("Gateway listening", slog.String("addr", listener.Addr().String()))
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}
	srv.RestartFunc = func() error {
		serverMu.Lock()
		defer serverMu.Unlock()
		if stopping {
			return fmt.Errorf("gateway is stopping")
		}
		replacement, restored, err := restartHTTPServer(httpServer, updater.RestartServer)
		if restored != nil {
			httpServer = replacement
			go serve(replacement, restored)
		} else {
			stopping = true
			if err != nil {
				select {
				case serverErr <- err:
				default:
				}
			}
		}
		return err
	}
	go serve(httpServer, ln)

	select {
	case err := <-serverErr:
		slog.Error("Server terminated with critical error", "error", err)
		return
	case <-done:
		slog.Info("Shutting down gracefully...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverMu.Lock()
	stopping = true
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("Forced shutdown", "error", err)
	}
	serverMu.Unlock()

	slog.Info("Gateway stopped")
}

func ensureAdminPassword(database *db.DB, explicitPassword string) {
	settings, err := database.GetSettings()
	if err != nil || settings == nil {
		return
	}

	// 1. Explicit password override from CLI flag or environment variable
	if explicitPassword != "" {
		settings.PasswordHash = auth.HashPassword(explicitPassword)
		if err := database.SaveSettings(settings); err != nil {
			slog.Error("Failed to save configured admin password", "error", err)
		} else {
			slog.Info("Admin password initialized from configuration flag/env")
		}
		return
	}

	// 2. Initial run on fresh database with no configured password
	if settings.PasswordHash == "" {
		initialPassword := auth.GenerateRandomPassword()
		settings.PasswordHash = auth.HashPassword(initialPassword)
		if err := database.SaveSettings(settings); err != nil {
			slog.Error("Failed to persist initial admin password", "error", err)
			return
		}

		// Print conspicuous banner to console for operator visibility
		fmt.Printf(`
========================================================================
[Cyrene Gateway] Initial Admin Password Generated:
  Password:  %s
Localhost defaults to local-first mode (no login required). This password
is reserved for remote WAN access or if you enable 'Require Login' in Settings.
========================================================================
`, initialPassword)
		slog.Info("Initial admin password generated", slog.String("password", initialPassword))
	}
}
