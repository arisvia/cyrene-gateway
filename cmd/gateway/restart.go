package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
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
