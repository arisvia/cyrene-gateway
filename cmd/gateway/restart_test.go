package main

import (
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestRestartFailureRestoresListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Addr: ln.Addr().String(), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})}
	defer srv.Close()
	go srv.Serve(ln)
	ready, err := http.Get("http://" + srv.Addr + "/health")
	if err != nil {
		t.Fatal(err)
	}
	ready.Body.Close()
	spawnErr := errors.New("cannot start successor")
	replacement, restored, err := restartHTTPServer(srv, func() error {
		probe, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			t.Errorf("listener was not released before spawn: %v", err)
		} else {
			probe.Close()
		}
		return spawnErr
	})
	if !errors.Is(err, spawnErr) {
		t.Fatalf("spawn failure lost: %v", err)
	}
	if replacement == nil || restored == nil {
		t.Fatal("spawn failure left gateway without a listener")
	}
	defer replacement.Close()
	go replacement.Serve(restored)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://" + srv.Addr + "/health")
	if err != nil {
		t.Fatalf("gateway did not recover after failed restart: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("restored handler status = %d", resp.StatusCode)
	}
}

func TestRestartFailureCannotRestoreOccupiedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Addr: ln.Addr().String(), Handler: http.NotFoundHandler()}
	defer srv.Close()
	go srv.Serve(ln)
	ready, err := http.Get("http://" + srv.Addr + "/health")
	if err != nil {
		t.Fatal(err)
	}
	ready.Body.Close()
	spawnErr := errors.New("spawn failed")
	var occupied net.Listener
	replacement, restored, err := restartHTTPServer(srv, func() error {
		occupied, err = net.Listen("tcp", srv.Addr)
		if err != nil {
			t.Fatal(err)
		}
		return spawnErr
	})
	defer occupied.Close()
	if !errors.Is(err, spawnErr) || replacement != nil || restored != nil {
		t.Fatalf("unrecoverable restart result: server=%v listener=%v error=%v", replacement, restored, err)
	}
}

func TestRestartSuccessReleasesPortBeforeSpawn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Addr: ln.Addr().String(), Handler: http.NotFoundHandler()}
	defer srv.Close()
	go srv.Serve(ln)
	ready, err := http.Get("http://" + srv.Addr + "/health")
	if err != nil {
		t.Fatal(err)
	}
	ready.Body.Close()
	replacement, restored, err := restartHTTPServer(srv, func() error {
		probe, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			return err
		}
		return probe.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	if replacement != nil || restored != nil {
		t.Fatal("successful handover must not reopen the parent listener")
	}
}
