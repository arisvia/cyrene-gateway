package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// auditRow returns the single request-detail row recorded for model, failing the
// test if the request left no trace at all.
func auditRow(t *testing.T, database *db.DB, providerID, modelName string) map[string]any {
	t.Helper()
	res, err := database.GetRequestDetails(db.RequestDetailFilter{Provider: providerID, Model: modelName, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("read request details: %v", err)
	}
	if len(res.Details) == 0 {
		t.Fatalf("no request-detail row recorded for %s/%s", providerID, modelName)
	}
	var row map[string]any
	if err := json.Unmarshal(res.Details[0], &row); err != nil {
		t.Fatalf("decode detail row: %v", err)
	}
	return row
}

// postChat sends one non-streaming chat request through the full handler stack.
func postChat(t *testing.T, srv *Server, modelName string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"model":      modelName,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		"max_tokens": 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	// Background catalog sync may still be running against the DB.
	t.Cleanup(srv.waitBackground)
	return w
}

// A request that no upstream ever served used to be invisible in 请求明细: the
// client got a JSON error and nothing was recorded, so an empty request log looked
// like "nothing was tried" instead of "the provider never answered" — precisely how
// a queue-stalled or credential-less provider gets misread as a routing bug.
func TestAbandonedRequestsAreAudited(t *testing.T) {
	t.Run("no active credentials", func(t *testing.T) {
		srv, database := setupTestServer(t)
		conn := &model.ProviderConnection{
			ID:       "opencode-off",
			Provider: "opencode",
			AuthType: "api-key",
			IsActive: false, // parked: the pool is empty for this provider
			Data:     model.ConnectionData{APIKey: "sk-test"},
		}
		if err := database.CreateConnection(conn); err != nil {
			t.Fatalf("create connection: %v", err)
		}

		w := postChat(t, srv, "opencode/audit-noauth")
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
		}

		row := auditRow(t, database, "opencode", "audit-noauth")
		if row["status"] != "503" {
			t.Errorf("status=%v, want 503", row["status"])
		}
		if reason, _ := row["error"].(string); reason == "" {
			t.Error("abandoned row carries no reason, so the panel cannot explain the failure")
		}
		if row["connectionId"] != "" {
			t.Errorf("connectionId=%v, want empty because no account was ever selected", row["connectionId"])
		}
		if row["endpoint"] != "/v1/chat/completions" {
			t.Errorf("endpoint=%v", row["endpoint"])
		}
	})

	t.Run("failover exhausted without an upstream response", func(t *testing.T) {
		srv, database := setupTestServer(t)
		conn := &model.ProviderConnection{
			ID:       "opencode-dead",
			Provider: "opencode",
			AuthType: "api-key",
			IsActive: true,
			// Port 9 (discard) refuses: the transport fails before any status line,
			// which is the shape of an upstream that never answered.
			Data: model.ConnectionData{APIKey: "sk-test", BaseURL: "http://127.0.0.1:9/v1"},
		}
		if err := database.CreateConnection(conn); err != nil {
			t.Fatalf("create connection: %v", err)
		}

		w := postChat(t, srv, "opencode/audit-dead")
		if w.Code == http.StatusOK {
			t.Fatalf("expected the dead upstream to fail the request: %s", w.Body.String())
		}

		row := auditRow(t, database, "opencode", "audit-dead")
		switch row["status"] {
		case "502", "503":
		default:
			t.Errorf("status=%v, want a failed status on the abandoned row", row["status"])
		}
		if ms, ok := row["latencyMs"].(float64); !ok || ms < 0 {
			t.Errorf("latencyMs=%v, want the wait duration so a queue stall is visible", row["latencyMs"])
		}
	})

	t.Run("canceled request is recorded as canceled", func(t *testing.T) {
		srv, database := setupTestServer(t)
		blocked := make(chan struct{})
		defer close(blocked)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-blocked
		}))
		defer upstream.Close()

		conn := &model.ProviderConnection{
			ID:       "opencode-hang",
			Provider: "opencode",
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: "sk-test", BaseURL: upstream.URL},
		}
		if err := database.CreateConnection(conn); err != nil {
			t.Fatalf("create connection: %v", err)
		}

		body, _ := json.Marshal(map[string]any{
			"model":      "opencode/audit-cancel",
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
			"max_tokens": 8,
		})
		// The client hangs up while the upstream is still thinking. Nothing is
		// written to the response, so the audit row is the only trace this turn
		// ever leaves.
		postChatCanceled(t, srv, body)
		row := auditRow(t, database, "opencode", "audit-cancel")
		if row["status"] != "canceled" {
			t.Errorf("status=%v, want \"canceled\" so the log distinguishes hang-up from failure", row["status"])
		}
		if reason, _ := row["error"].(string); !strings.Contains(reason, "canceled") {
			t.Errorf("error=%q, want the reason to name the cancellation", reason)
		}
	})
}

// postChatCanceled issues a chat request whose client context is canceled before
// the handler can finish, mirroring a user closing the tab mid-wait.
func postChatCanceled(t *testing.T, srv *Server, body []byte) {
	t.Helper()
	req := newLocalRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Handler.ServeHTTP(w, req)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not return after the client canceled")
	}
}
