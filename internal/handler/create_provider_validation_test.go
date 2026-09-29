package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// TestHandleCreateProviderValidation covers the server-side hardening of
// POST /api/providers: required provider, apiKey-required for api-key auth,
// and the per provider+authType duplicate guard (409).
func TestHandleCreateProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    int
		preseed bool // pre-create an openai api-key connection for the duplicate case
	}{
		{
			name: "missing provider is rejected",
			body: `{"name":"no provider","data":{"apiKey":"sk-x"}}`,
			want: http.StatusBadRequest,
		},
		{
			name: "api-key auth without apiKey is rejected",
			body: `{"provider":"openai","authType":"api-key","data":{}}`,
			want: http.StatusBadRequest,
		},
		{
			name: "valid api-key connection is created",
			body: `{"provider":"deepseek","authType":"api-key","data":{"apiKey":"sk-ds"}}`,
			want: http.StatusCreated,
		},
		{
			name: "oauth connection without apiKey is allowed",
			body: `{"provider":"github","authType":"oauth","data":{}}`,
			want: http.StatusCreated,
		},
		{
			name:    "duplicate provider+authType is rejected",
			body:    `{"provider":"openai","authType":"api-key","data":{"apiKey":"sk-dup"}}`,
			want:    http.StatusConflict,
			preseed: true,
		},
		{
			name: "same provider different authType is allowed",
			body: `{"provider":"openai","authType":"oauth","data":{}}`,
			want: http.StatusCreated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, database := setupTestServer(t)

			if tc.preseed {
				conn := &model.ProviderConnection{
					Provider: "openai",
					AuthType: "api-key",
					IsActive: true,
					Data:     model.ConnectionData{APIKey: "sk-seed"},
				}
				if err := database.CreateConnection(conn); err != nil {
					t.Fatalf("preseed connection: %v", err)
				}
			}

			req := newLocalRequest("POST", "/api/providers", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

// Connections are routed and listed by canonical provider id, so a row stored
// under its shorthand would be invisible to ListConnectionsByProvider.
func TestProviderWriteBoundaryCanonicalizesAlias(t *testing.T) {
	srv, database := setupTestServer(t)

	w := httptest.NewRecorder()
	req := newLocalRequest(http.MethodPost, "/api/providers",
		strings.NewReader(`{"provider":"cc","authType":"api-key","data":{"apiKey":"sk-cc"}}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}

	stored, err := database.ListConnectionsByProvider("claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("the alias-stored connection is invisible to its canonical provider: %+v", stored)
	}
	if got := stored[0].Provider; got != "claude" {
		t.Errorf("created provider = %q, want the canonical id", got)
	}

	w = httptest.NewRecorder()
	req = newLocalRequest(http.MethodPut, "/api/providers/"+stored[0].ID,
		strings.NewReader(`{"provider":"ds"}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", w.Code, w.Body.String())
	}
	updated, err := database.ListConnectionsByProvider("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].ID != stored[0].ID {
		t.Fatalf("update stored the shorthand verbatim: %+v", updated)
	}
}

// Reads keyed by provider string must reach the same connection whether the
// caller used the shorthand or the canonical id: without canonicalization the
// alias simply returns an empty list, which no client can tell apart from
// "this provider has no connection".
func TestProviderReadBoundaryCanonicalizesAlias(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID:       "conn-claude",
		Provider: "claude",
		AuthType: "oauth",
		Name:     "Claude Account",
		IsActive: true,
		Data:     model.ConnectionData{AccessToken: "at-claude"},
	}); err != nil {
		t.Fatalf("preseed connection: %v", err)
	}

	byAlias := httptest.NewRecorder()
	srv.Handler.ServeHTTP(byAlias, newLocalRequest(http.MethodGet, "/api/oauth/cc/status", nil))
	byID := httptest.NewRecorder()
	srv.Handler.ServeHTTP(byID, newLocalRequest(http.MethodGet, "/api/oauth/claude/status", nil))

	if byAlias.Code != http.StatusOK || byID.Code != http.StatusOK {
		t.Fatalf("status alias=%d id=%d", byAlias.Code, byID.Code)
	}
	if !strings.Contains(byAlias.Body.String(), "conn-claude") {
		t.Fatalf("alias read found no connection: %s", byAlias.Body.String())
	}
	if byAlias.Body.String() != byID.Body.String() {
		t.Errorf("alias read diverged from canonical: alias=%s id=%s", byAlias.Body.String(), byID.Body.String())
	}
}
