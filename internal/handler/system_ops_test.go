package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
)

func TestSystemOpsHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-sysops.sqlite")

	testDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer testDB.Close()

	cfg := &config.Config{
		DBPath:  dbPath,
		DataDir: tmpDir,
	}

	server := NewServer(testDB, cfg)

	// Test GET /api/system/stats
	t.Run("GET /api/system/stats", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/system/stats", nil)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var stats map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
			t.Fatalf("failed to unmarshal stats: %v", err)
		}

		if stats["os"] == "" || stats["arch"] == "" {
			t.Errorf("missing os or arch in stats")
		}
		if stats["memory"] == nil {
			t.Errorf("missing memory in stats")
		}
	})

	// Test POST /api/system/maintenance checkpoint
	t.Run("POST /api/system/maintenance checkpoint", func(t *testing.T) {
		payload := bytes.NewBufferString(`{"action":"checkpoint"}`)
		req := httptest.NewRequest("POST", "/api/system/maintenance", payload)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// Test POST /api/system/maintenance vacuum
	t.Run("POST /api/system/maintenance vacuum", func(t *testing.T) {
		payload := bytes.NewBufferString(`{"action":"vacuum"}`)
		req := httptest.NewRequest("POST", "/api/system/maintenance", payload)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// Test POST /api/system/maintenance prune_logs
	t.Run("POST /api/system/maintenance prune_logs", func(t *testing.T) {
		payload := bytes.NewBufferString(`{"action":"prune_logs","retentionDays":14}`)
		req := httptest.NewRequest("POST", "/api/system/maintenance", payload)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// Test POST /api/system/maintenance invalid action
	t.Run("POST /api/system/maintenance invalid", func(t *testing.T) {
		payload := bytes.NewBufferString(`{"action":"invalid"}`)
		req := httptest.NewRequest("POST", "/api/system/maintenance", payload)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	// Test POST /api/system/restart
	t.Run("POST /api/system/restart", func(t *testing.T) {
		restarted := make(chan struct{})
		server.onRestart = func() {
			close(restarted)
		}

		req := httptest.NewRequest("POST", "/api/system/restart", nil)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		select {
		case <-restarted:
			// Success
		case <-time.After(1 * time.Second):
			t.Errorf("timeout waiting for onRestart hook to be called")
		}
	})

	// Test POST /api/system/rollback without backup
	t.Run("POST /api/system/rollback missing backup", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/system/rollback", nil)
		rec := httptest.NewRecorder()
		server.Router.ServeHTTP(rec, req)

		// Expect 500 because no .old exists
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 error when no backup exists, got %d", rec.Code)
		}
	})

	// Test Docker environment blocks update and restart
	t.Run("Docker environment guards", func(t *testing.T) {
		os.Setenv("CYRENE_IN_DOCKER", "1")
		defer os.Unsetenv("CYRENE_IN_DOCKER")

		// Update should fail with 400
		reqUpdate := httptest.NewRequest("POST", "/api/system/update", nil)
		recUpdate := httptest.NewRecorder()
		server.Router.ServeHTTP(recUpdate, reqUpdate)
		if recUpdate.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request in docker for update, got %d", recUpdate.Code)
		}

		// Restart should fail with 400
		reqRestart := httptest.NewRequest("POST", "/api/system/restart", nil)
		recRestart := httptest.NewRecorder()
		server.Router.ServeHTTP(recRestart, reqRestart)
		if recRestart.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request in docker for restart, got %d", recRestart.Code)
		}
	})

	_ = os.DevNull
}
