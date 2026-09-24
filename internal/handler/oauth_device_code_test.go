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
	req := newLocalRequest("POST", "/api/oauth/codebuddy-cn/device-code", nil)
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
	pollReq := newLocalRequest("POST", "/api/oauth/codebuddy-cn/device-code/poll", strings.NewReader(pollReqBody))
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
	pollSnakeReq := newLocalRequest("POST", "/api/oauth/codebuddy-cn/device-code/poll", strings.NewReader(pollSnakeBody))
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
func TestOAuthImport_PriorityAndNamePersistence(t *testing.T) {
	srv, database := setupTestServer(t)

	// Test POST /api/oauth/cursor/import with custom name and priority
	reqBody := `{"accessToken":"eyJhbGciOiJub25lIn0.eyJzdWIiOiJjdXJzb3ItdXNlci0xIiwiZW1haWwiOiJjdXJzb3JAdGVzdC5jb20ifQ.","name":"My Second Cursor","priority":25}`
	req := newLocalRequest("POST", "/api/oauth/cursor/import", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("import returned status %d, body: %s", w.Code, w.Body.String())
	}

	conns, err := database.ListConnectionsByProvider("cursor")
	if err != nil || len(conns) == 0 {
		t.Fatalf("expected cursor connection in db, got err=%v conns=%+v", err, conns)
	}
	conn := conns[0]
	if conn.Priority != 25 {
		t.Errorf("expected connection priority 25, got %d", conn.Priority)
	}
	if conn.Name != "My Second Cursor" {
		t.Errorf("expected connection name 'My Second Cursor', got %q", conn.Name)
	}
}
func TestOAuthDeviceCode_QoderPriorityAndName(t *testing.T) {
	srv, database := setupTestServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "poll") {
			w.Write([]byte(`{"token":"dt-mock-123","refresh_token":"rt-mock-123","user_id":"u-999","expires_at":1800000000000}`))
			return
		}
		if strings.Contains(r.URL.Path, "userinfo") {
			w.Write([]byte(`{"code":0,"data":{"username":"qoder-dev","email":"qoder@test.com"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	origPollURL := provider.QoderDeviceTokenURL
	origUserinfoURL := provider.QoderUserinfoURL
	provider.QoderDeviceTokenURL = ts.URL + "/poll"
	provider.QoderUserinfoURL = ts.URL + "/userinfo"
	defer func() {
		provider.QoderDeviceTokenURL = origPollURL
		provider.QoderUserinfoURL = origUserinfoURL
	}()

	reqBody := `{"nonce":"n1","codeVerifier":"cv1","machineId":"m1","name":"Qoder Secondary","priority":35}`
	req := newLocalRequest("POST", "/api/oauth/qoder/device-code/poll", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("poll returned status %d, body: %s", w.Code, w.Body.String())
	}

	conns, err := database.ListConnectionsByProvider("qoder")
	if err != nil || len(conns) == 0 {
		t.Fatalf("expected qoder connection in db, got err=%v conns=%+v", err, conns)
	}
	conn := conns[0]
	if conn.Priority != 35 {
		t.Errorf("expected connection priority 35, got %d", conn.Priority)
	}
	if conn.Name != "Qoder Secondary" {
		t.Errorf("expected connection name 'Qoder Secondary', got %q", conn.Name)
	}
}
