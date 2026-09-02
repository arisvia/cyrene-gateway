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

			req := httptest.NewRequest("POST", "/api/providers", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
