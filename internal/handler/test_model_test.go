package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestTestModelOpenCodeFreeModel(t *testing.T) {
	var capturedAuth string
	var capturedUA string
	var capturedClient string
	var capturedSession string

	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedUA = r.Header.Get("User-Agent")
		capturedClient = r.Header.Get("x-opencode-client")
		capturedSession = r.Header.Get("x-opencode-session")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer mockUpstream.Close()

	srv, database := setupTestServer(t)
	conn := &model.ProviderConnection{
		ID:       "oc-test-conn",
		Provider: "opencode",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "my-unfunded-secret-key",
			BaseURL: mockUpstream.URL,
		},
	}
	database.CreateConnection(conn)

	// Test free model: big-pickle -> must route with Bearer public and OpenCode fingerprint headers
	reqBody := `{"model":"opencode/big-pickle","connectionId":"oc-test-conn"}`
	req := httptest.NewRequest("POST", "/api/models/test", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if ok, _ := res["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %v (error: %v)", res["ok"], res["error"])
	}

	if capturedAuth != "Bearer public" {
		t.Errorf("expected Bearer public for free model test, got %q", capturedAuth)
	}
	if capturedUA != "opencode/1.18.31" {
		t.Errorf("expected User-Agent opencode/1.18.31, got %q", capturedUA)
	}
	if capturedClient != "desktop" {
		t.Errorf("expected x-opencode-client desktop, got %q", capturedClient)
	}
	if !strings.HasPrefix(capturedSession, "ses_") || len(capturedSession) != 30 {
		t.Errorf("expected 30-char canonical session, got %q", capturedSession)
	}

	// Test paid model: deepseek-v4-pro -> must route with configured APIKey
	reqPaidBody := `{"model":"opencode/deepseek-v4-pro","connectionId":"oc-test-conn"}`
	reqPaid := httptest.NewRequest("POST", "/api/models/test", strings.NewReader(reqPaidBody))
	reqPaid.Header.Set("Content-Type", "application/json")
	wPaid := httptest.NewRecorder()

	srv.Handler.ServeHTTP(wPaid, reqPaid)
	if wPaid.Code != http.StatusOK {
		t.Fatalf("expected status 200 for paid model, got %d: %s", wPaid.Code, wPaid.Body.String())
	}
	if capturedAuth != "Bearer my-unfunded-secret-key" {
		t.Errorf("expected Bearer my-unfunded-secret-key for paid model, got %q", capturedAuth)
	}
}
