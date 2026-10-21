package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// newHTTPServer configures the listener for an LLM gateway: uploads can be
// large (a long conversation plus tool schemas easily exceeds 1MB) and
// responses are long-lived SSE streams.
//
// ReadTimeout is deliberately generous rather than tight: it covers the whole
// request body, and a slow client behind a tunnel needs more than the old 30s
// to push a multi-megabyte conversation. Slowloris is still bounded by
// ReadHeaderTimeout, and MaxBytesReader caps the body itself.
//
// WriteTimeout is 0 (no absolute deadline) because a fixed deadline kills
// legitimate long streams — a queued request that later starts generating can
// run for minutes. Hung streams are detected per-read by
// provider.NewStallTimeoutReader instead, which is the correct signal: it fires
// on silence, not on total elapsed time.
func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
	}
}

func restartHTTPServer(current *http.Server, spawn func() error) (*http.Server, net.Listener, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	shutdownErr := current.Shutdown(ctx)
	if err := spawn(); err != nil {
		ln, listenErr := net.Listen("tcp", current.Addr)
		if listenErr != nil {
			return nil, nil, errors.Join(err, shutdownErr, fmt.Errorf("restore listener: %w", listenErr))
		}
		return newHTTPServer(current.Addr, current.Handler), ln, err
	}
	return nil, nil, nil
}
