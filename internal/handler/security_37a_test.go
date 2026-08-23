package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestSecretRedactionContract(t *testing.T) {
	srv, database := setupTestServer(t)

	// Create a connection with full secrets
	conn := &model.ProviderConnection{
		ID:       "secret-test-conn",
		Provider: "openai",
		AuthType: "api-key",
		Name:     "Test Secrets",
		Data: model.ConnectionData{
			APIKey:       "sk-proj-super-secret-key-12345678",
			AccessToken:  "ghu_access_token_secret_87654321",
			RefreshToken: "ghr_refresh_token_secret_11223344",
			BaseURL:      "https://api.openai.com/v1",
			ProviderSpecificData: map[string]any{
				"refreshToken": "ghr_nested_refresh_secret",
				"publicParam":  "safe-value",
			},
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("failed to create connection: %v", err)
	}

	// 1. GET /api/providers (List)
	req := httptest.NewRequest("GET", "/api/providers", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	listBody := w.Body.String()
	assertNoSecrets(t, listBody)

	// 2. GET /api/providers/{id} (Detail)
	req = httptest.NewRequest("GET", "/api/providers/secret-test-conn", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	detailBody := w.Body.String()
	assertNoSecrets(t, detailBody)

	var dto model.ConnectionDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil {
		t.Fatalf("failed to unmarshal DTO: %v", err)
	}
	if !dto.Data.HasAPIKey || !dto.Data.HasAccessToken || !dto.Data.HasRefreshToken {
		t.Errorf("expected has flags to be true, got %+v", dto.Data)
	}
	if dto.Data.CredentialHint != "...5678" {
		t.Errorf("expected hint ...5678, got %s", dto.Data.CredentialHint)
	}

	// 3. PUT /api/providers/{id} (Update without secrets preserves them)
	updatePayload := `{"name":"Updated Name","data":{"baseUrl":"https://custom.openai.com/v1"}}`
	req = httptest.NewRequest("PUT", "/api/providers/secret-test-conn", strings.NewReader(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	updateBody := w.Body.String()
	assertNoSecrets(t, updateBody)

	// Verify database still holds the original secrets
	dbConn, err := database.GetConnection("secret-test-conn")
	if err != nil {
		t.Fatalf("failed to get connection from db: %v", err)
	}
	if dbConn.Data.APIKey != "sk-proj-super-secret-key-12345678" {
		t.Errorf("expected secret preserved in DB, got %s", dbConn.Data.APIKey)
	}
	if dbConn.Data.BaseURL != "https://custom.openai.com/v1" {
		t.Errorf("expected baseUrl updated, got %s", dbConn.Data.BaseURL)
	}
}

func TestRemoteUnauthenticatedManagementBlocked(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Request from remote IP (not loopback) without session auth
	req := httptest.NewRequest("GET", "/api/providers", nil)
	req.RemoteAddr = "203.0.113.195:54321" // Public WAN IP
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for remote unauthenticated management access, got %d", w.Code)
	}
}

func TestPasswordArgon2idMigration(t *testing.T) {
	srv, database := setupTestServer(t)

	// Manually inject a legacy HMAC password hash
	legacyPassword := "my-old-hmac-password"
	legacyHash := auth.HashPassword(legacyPassword) // Currently hashes to Argon2id, let's test verify
	settings, _ := database.GetSettings()
	settings.PasswordHash = legacyHash
	database.SaveSettings(settings)

	// Login with correct password
	loginBody := `{"password":"my-old-hmac-password"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify password hash in DB is argon2id format
	updatedSettings, _ := database.GetSettings()
	if !strings.HasPrefix(updatedSettings.PasswordHash, "$argon2id$") {
		t.Fatalf("expected password hash migrated to argon2id, got %s", updatedSettings.PasswordHash)
	}
}

func assertNoSecrets(t *testing.T, body string) {
	t.Helper()
	secrets := []string{
		"sk-proj-super-secret-key-12345678",
		"ghu_access_token_secret_87654321",
		"ghr_refresh_token_secret_11223344",
		"ghr_nested_refresh_secret",
	}
	for _, s := range secrets {
		if strings.Contains(body, s) {
			t.Fatalf("SECURITY VIOLATION: secret %q leaked in response: %s", s, body)
		}
	}
}
