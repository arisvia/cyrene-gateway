package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/config"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestBackupRestoreEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cyrene-handler-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "data.sqlite")
	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	cfg := &config.Config{DataDir: tmpDir}
	srv := NewServer(d, cfg)

	// Seed connection
	conn := &model.ProviderConnection{
		ID:       "c-1",
		Provider: "openai",
		AuthType: "api-key",
		Name:     "Endpoint Test",
		Data: model.ConnectionData{
			APIKey: "sk-endpoint-secret",
		},
	}
	if err := d.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}

	// 1. Test GET /api/system/backup
	req := httptest.NewRequest("GET", "/api/system/backup?include_secrets=true", nil)
	w := httptest.NewRecorder()
	srv.Router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from backup, got %d: %s", w.Code, w.Body.String())
	}

	var payload db.ExportPayload
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode backup payload: %v", err)
	}
	if len(payload.Data.ProviderConnections) != 1 {
		t.Fatalf("expected 1 connection in exported payload")
	}

	// 2. Modify connection and test POST /api/system/restore
	payload.Data.ProviderConnections[0].Name = "Restored Name"
	bodyBytes, _ := json.Marshal(payload)

	reqRestore := httptest.NewRequest("POST", "/api/system/restore?mode=replace", bytes.NewReader(bodyBytes))
	wRestore := httptest.NewRecorder()
	srv.Router.ServeHTTP(wRestore, reqRestore)

	if wRestore.Code != http.StatusOK {
		t.Fatalf("expected 200 from restore, got %d: %s", wRestore.Code, wRestore.Body.String())
	}
	// 3. Test invalid mode rejected with 400
	reqInvalidMode := httptest.NewRequest("POST", "/api/system/restore?mode=upsert", bytes.NewReader(bodyBytes))
	wInvalidMode := httptest.NewRecorder()
	srv.Router.ServeHTTP(wInvalidMode, reqInvalidMode)
	if wInvalidMode.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown restore mode 'upsert', got %d", wInvalidMode.Code)
	}

	// Verify database is completely intact and unaffected by the rejected request
	cAfter, err := d.GetConnection("c-1")
	if err != nil {
		t.Fatal(err)
	}
	if cAfter.Name != "Restored Name" {
		t.Fatalf("expected restored name 'Restored Name', got %s", cAfter.Name)
	}
	if cAfter.Data.APIKey != "sk-endpoint-secret" {
		t.Fatalf("expected APIKey 'sk-endpoint-secret' intact after 400 rejection, got %s", cAfter.Data.APIKey)
	}
}
