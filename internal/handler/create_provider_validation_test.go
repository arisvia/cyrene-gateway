package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
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
		{
			name: "unknown authType is rejected",
			body: `{"provider":"openai","authType":"no-auth","data":{"apiKey":"sk-x"}}`,
			want: http.StatusBadRequest,
		},
		{
			// A credential-less row is selectable by the candidate query but has
			// nothing to send upstream, and nothing repairs it at startup now.
			name: "none authType is rejected",
			body: `{"provider":"opencode","authType":"none","data":{}}`,
			want: http.StatusBadRequest,
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

// Admin reads name a provider by its canonical id only: the console sends
// registry ids, so a shorthand reaching /api/* means a hand-built request that
// should surface as "no such provider" instead of quietly aliasing to a second
// identity for the same upstream.
func TestAdminReadRejectsProviderAlias(t *testing.T) {
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

	if byID.Code != http.StatusOK {
		t.Fatalf("canonical status=%d body=%s", byID.Code, byID.Body.String())
	}
	if !strings.Contains(byID.Body.String(), "conn-claude") {
		t.Fatalf("canonical read lost its own connection: %s", byID.Body.String())
	}
	if strings.Contains(byAlias.Body.String(), "conn-claude") {
		t.Errorf("alias read reached the connection, want a canonical-id-only surface: %s", byAlias.Body.String())
	}
}

// The explicit provider override is compared against the model prefix, so an
// alias in one field and the canonical id in the other must still collapse to
// the bare model instead of forwarding "openai/text-emb-3" upstream.
func TestEmbeddingsOverrideTrimsCanonicalPrefix(t *testing.T) {
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer upstream.Close()

	srv, database := setupTestServer(t)
	defer database.Close()

	orig, hasOrig := provider.Registry["openai"]
	info := orig
	info.BaseURL = upstream.URL
	provider.Registry["openai"] = info
	defer func() {
		if hasOrig {
			provider.Registry["openai"] = orig
		} else {
			delete(provider.Registry, "openai")
		}
	}()

	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "oai-embed", Provider: "openai", AuthType: "api-key",
		Name: "Embed Test", IsActive: true,
		Data: model.ConnectionData{APIKey: "sk-test"},
	}); err != nil {
		t.Fatalf("preseed connection: %v", err)
	}

	req := newLocalRequest(http.MethodPost, "/v1/embeddings",
		strings.NewReader(`{"provider":"oai","model":"openai/text-emb-3","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if gotModel != "text-emb-3" {
		t.Errorf("upstream model = %q, want the prefix trimmed to the bare model", gotModel)
	}
}
