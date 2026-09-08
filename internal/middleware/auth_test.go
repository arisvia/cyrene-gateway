package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func setupTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "cyrene-mw-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	database, err := db.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("failed to init db: %v", err)
	}
	cleanup := func() {
		database.Close()
		os.RemoveAll(dir)
	}
	return database, cleanup
}

func TestAPIKeyAuth_OpenGatewayMode(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Settings: RequireAPIKey = false (default open mode)
	st, _ := database.GetSettings()
	st.RequireAPIKey = false
	_ = database.SaveSettings(st)

	// Create one valid active key with signature
	keyStr := auth.GenerateAPIKey()
	validKey := &model.APIKey{
		ID:        "key-open-1",
		Key:       keyStr,
		Name:      "Valid User",
		IsActive:  true,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}
	if err := database.CreateAPIKey(validKey); err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	var capturedKey *model.APIKey
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = auth.APIKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Non-v1 path passes through
	{
		capturedKey = nil
		req := httptest.NewRequest("GET", "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for non-v1 path, got %d", rec.Code)
		}
	}

	// Case 2: Open gateway + no header -> 200 OK (anonymous)
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for anonymous in open mode, got %d", rec.Code)
		}
		if capturedKey != nil {
			t.Errorf("expected nil capturedKey, got %v", capturedKey)
		}
	}

	// Case 3: Open gateway + garbage Bearer key -> 200 OK (pass-through as anonymous, NOT 401)
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer invalid-garbage-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 pass-through for garbage key in open mode, got %d", rec.Code)
		}
		if capturedKey != nil {
			t.Errorf("expected nil capturedKey, got %v", capturedKey)
		}
	}

	// Case 4: Open gateway + valid Bearer key -> 200 OK with Key injected into Context
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+validKey.Key)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for valid key, got %d", rec.Code)
		}
		if capturedKey == nil || capturedKey.ID != validKey.ID {
			t.Errorf("expected capturedKey ID %s, got %v", validKey.ID, capturedKey)
		}
	}
}

func TestAPIKeyAuth_StrictGatewayMode(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Settings: RequireAPIKey = true
	st, _ := database.GetSettings()
	st.RequireAPIKey = true
	_ = database.SaveSettings(st)

	keyStr := auth.GenerateAPIKey()
	validKey := &model.APIKey{
		ID:        "key-strict-1",
		Key:       keyStr,
		Name:      "Strict User",
		IsActive:  true,
		ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}
	if err := database.CreateAPIKey(validKey); err != nil {
		t.Fatalf("failed to create key: %v", err)
	}

	var capturedKey *model.APIKey
	handler := APIKeyAuth(database)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = auth.APIKeyFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Strict mode + no header -> 401
	{
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for missing key in strict mode, got %d", rec.Code)
		}
	}

	// Case 2: Strict mode + invalid key -> 401
	{
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer cg-invalidkey123")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid key in strict mode, got %d", rec.Code)
		}
	}

	// Case 3: Strict mode + valid key -> 200 with context
	{
		capturedKey = nil
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer "+validKey.Key)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 for valid key in strict mode, got %d", rec.Code)
		}
		if capturedKey == nil || capturedKey.ID != validKey.ID {
			t.Errorf("expected capturedKey ID %s, got %v", validKey.ID, capturedKey)
		}
	}
}
