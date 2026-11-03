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

// Connections are routed and listed by provider id, and the write boundary no
// longer rewrites it: there is exactly one spelling per provider.
func TestProviderWriteBoundaryStoresProviderVerbatim(t *testing.T) {
	srv, database := setupTestServer(t)

	w := httptest.NewRecorder()
	req := newLocalRequest(http.MethodPost, "/api/providers",
		strings.NewReader(`{"provider":"claude","authType":"api-key","data":{"apiKey":"sk-cc"}}`))
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
		t.Fatalf("connection not found under its provider id: %+v", stored)
	}
	if got := stored[0].Provider; got != "claude" {
		t.Errorf("created provider = %q, want the id verbatim", got)
	}

	w = httptest.NewRecorder()
	req = newLocalRequest(http.MethodPut, "/api/providers/"+stored[0].ID,
		strings.NewReader(`{"provider":"deepseek"}`))
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
		t.Fatalf("update did not move the connection to deepseek: %+v", updated)
	}
}

// The explicit provider override is compared against the model prefix, so the
// two spellings must still collapse to the bare model instead of forwarding
// "openai/text-emb-3" upstream.
func TestEmbeddingsOverrideTrimsProviderPrefix(t *testing.T) {
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
		strings.NewReader(`{"provider":"openai","model":"openai/text-emb-3","input":"hi"}`))
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
