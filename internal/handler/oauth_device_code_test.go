package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// TestOAuthDeviceCode_WireFormat verifies that /api/oauth/{provider}/device-code
// emits camelCase JSON fields matching WebUI contracts (deviceCode, verificationUri)
// and that /api/oauth/{provider}/device-code/poll accepts deviceCode without 400.
func TestOAuthDeviceCode_WireFormat(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Mock CodeBuddy CN upstream server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "state") {
			w.Write([]byte(`{"code":0,"msg":"OK","data":{"state":"mock-state-abc","authUrl":"https://copilot.tencent.com/login?platform=CLI&state=mock-state-abc"}}`))
			return
		}
		if strings.Contains(r.URL.Path, "token") {
			// Return codebuddy pending code 11217
			w.Write([]byte(`{"code":11217,"msg":"login ing..."}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	orig := provider.Registry["codebuddy-cn"]
	defer func() { provider.Registry["codebuddy-cn"] = orig }()

	mockInfo := orig
	mockInfo.DeviceCodeURL = ts.URL + "/state"
	mockInfo.TokenURL = ts.URL + "/token"
	provider.Registry["codebuddy-cn"] = mockInfo

	// 1. Test POST /api/oauth/codebuddy-cn/device-code
	req := httptest.NewRequest("POST", "/api/oauth/codebuddy-cn/device-code", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("device-code returned status %d, body: %s", w.Code, w.Body.String())
	}
	var bodyMap map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &bodyMap); err != nil {
		t.Fatalf("failed to decode device-code json: %v", err)
	}
	if bodyMap["deviceCode"] != "mock-state-abc" {
		t.Errorf("expected camelCase deviceCode in response, got: %v", bodyMap["deviceCode"])
	}
	if bodyMap["verificationUri"] != "https://copilot.tencent.com/login?platform=CLI&state=mock-state-abc" {
		t.Errorf("expected camelCase verificationUri in response, got: %v", bodyMap["verificationUri"])
	}
	if _, ok := bodyMap["device_code"]; ok {
		t.Errorf("unexpected snake_case device_code in response: %v", bodyMap)
	}
	if _, ok := bodyMap["verification_uri"]; ok {
		t.Errorf("unexpected snake_case verification_uri in response: %v", bodyMap)
	}
	pollReqBody := `{"deviceCode":"mock-state-abc"}`
	pollReq := httptest.NewRequest("POST", "/api/oauth/codebuddy-cn/device-code/poll", strings.NewReader(pollReqBody))
	pollReq.Header.Set("Content-Type", "application/json")
	pollW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(pollW, pollReq)

	if pollW.Code != http.StatusOK {
		t.Fatalf("device-code/poll returned status %d, body: %s", pollW.Code, pollW.Body.String())
	}
	pollBodyStr := pollW.Body.String()
	if !strings.Contains(pollBodyStr, `"pending":true`) {
		t.Errorf("expected pending:true in poll response, got: %s", pollBodyStr)
	}

	// 3. Test POST /api/oauth/codebuddy-cn/device-code/poll with snake_case device_code fallback
	pollSnakeBody := `{"device_code":"mock-state-abc"}`
	pollSnakeReq := httptest.NewRequest("POST", "/api/oauth/codebuddy-cn/device-code/poll", strings.NewReader(pollSnakeBody))
	pollSnakeReq.Header.Set("Content-Type", "application/json")
	pollSnakeW := httptest.NewRecorder()
	srv.Handler.ServeHTTP(pollSnakeW, pollSnakeReq)

	if pollSnakeW.Code != http.StatusOK {
		t.Fatalf("device-code/poll with snake_code returned status %d, body: %s", pollSnakeW.Code, pollSnakeW.Body.String())
	}
	if !strings.Contains(pollSnakeW.Body.String(), `"pending":true`) {
		t.Errorf("expected pending:true in snake fallback poll response, got: %s", pollSnakeW.Body.String())
	}
}
