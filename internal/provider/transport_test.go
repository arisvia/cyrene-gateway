package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestResolveTransportOpenAIBearer(t *testing.T) {
	p := Registry["openai"]
	conn := &model.ProviderConnection{
		Provider: "openai", AuthType: "api-key",
		Data: model.ConnectionData{APIKey: "sk-test"},
	}
	tr := ResolveTransport(p, p.BaseURL, p.APIType, conn)

	if tr.Auth.Scheme != AuthBearer {
		t.Errorf("expected bearer scheme, got %q", tr.Auth.Scheme)
	}
	if tr.Auth.Header != "Authorization" {
		t.Errorf("expected Authorization header, got %q", tr.Auth.Header)
	}
	if tr.Format != "openai" {
		t.Errorf("expected openai format, got %q", tr.Format)
	}
}

func TestResolveTransportAnthropicRawXAPIKey(t *testing.T) {
	p := Registry["anthropic"]
	conn := &model.ProviderConnection{
		Provider: "anthropic", AuthType: "api-key",
		Data: model.ConnectionData{APIKey: "sk-ant-test"},
	}
	tr := ResolveTransport(p, p.BaseURL, p.APIType, conn)

	if tr.Auth.Scheme != AuthRaw {
		t.Errorf("expected raw scheme, got %q", tr.Auth.Scheme)
	}
	if tr.Auth.Header != "x-api-key" {
		t.Errorf("expected x-api-key header, got %q", tr.Auth.Header)
	}
	// anthropic registry declares anthropic-version + Anthropic-Beta headers
	if tr.Headers["anthropic-version"] != "2023-06-01" {
		t.Errorf("expected anthropic-version header, got %q", tr.Headers["anthropic-version"])
	}
	if tr.Headers["Anthropic-Beta"] == "" {
		t.Error("expected Anthropic-Beta header to be set")
	}
}

func TestResolveTransportGeminiQuery(t *testing.T) {
	// With no API key (e.g. OAuth path), gemini falls back to the format-derived
	// query default. Phase 31 moved api-key auth to the x-goog-api-key header
	// (see TestResolveTransportGeminiAPIKeyHeader).
	p := Registry["gemini"]
	conn := &model.ProviderConnection{
		Provider: "gemini", AuthType: "oauth",
		Data: model.ConnectionData{AccessToken: "ya29-token"},
	}
	tr := ResolveTransport(p, p.BaseURL, p.APIType, conn)

	// OAuth connection has no API key, but the registry still declares an
	// explicit raw auth on x-goog-api-key; the token is injected there.
	if tr.Auth.Scheme != AuthRaw {
		t.Errorf("expected raw scheme, got %q", tr.Auth.Scheme)
	}
	if tr.Auth.Header != "x-goog-api-key" {
		t.Errorf("expected x-goog-api-key header, got %q", tr.Auth.Header)
	}
}

// Kimi OAuth path: declared x-api-key raw auth + kimiHeaders hook + ?beta=true suffix.
func TestResolveTransportKimiOAuth(t *testing.T) {
	p := Registry["kimi"]
	conn := &model.ProviderConnection{
		Provider: "kimi", AuthType: "oauth",
		Data: model.ConnectionData{
			AccessToken:          "kimi-token",
			ProviderSpecificData: map[string]any{"deviceId": "dev-123"},
		},
	}
	baseURL, apiType := p.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	tr := ResolveTransport(p, baseURL, apiType, conn)

	if tr.Auth.Scheme != AuthRaw {
		t.Errorf("expected raw scheme for kimi OAuth, got %q", tr.Auth.Scheme)
	}
	if tr.Auth.Header != "x-api-key" {
		t.Errorf("expected x-api-key header, got %q", tr.Auth.Header)
	}
	if len(tr.Auth.Hooks) != 1 || tr.Auth.Hooks[0] != "kimiHeaders" {
		t.Errorf("expected kimiHeaders hook, got %v", tr.Auth.Hooks)
	}
	if tr.URLSuffix != "?beta=true" {
		t.Errorf("expected ?beta=true suffix, got %q", tr.URLSuffix)
	}
}

// Kimi apikey path: routed to the moonshot OpenAI endpoint, so it must derive
// plain Bearer auth (NOT the claude x-api-key transport). This is the dual-auth
// guarantee of Phase 30.
func TestResolveTransportKimiAPIKeyDualAuth(t *testing.T) {
	p := Registry["kimi"]
	conn := &model.ProviderConnection{
		Provider: "kimi", AuthType: "api-key",
		Data: model.ConnectionData{APIKey: "sk-moonshot"},
	}
	baseURL, apiType := p.EffectiveBaseURL(conn.AuthType, conn.Data.APIKey != "")
	if baseURL != p.ApiKeyBaseURL {
		t.Fatalf("expected apikey base URL override, got %q", baseURL)
	}
	tr := ResolveTransport(p, baseURL, apiType, conn)

	if tr.Auth.Scheme != AuthBearer {
		t.Errorf("expected bearer scheme for kimi apikey path, got %q", tr.Auth.Scheme)
	}
	if tr.Format != "openai" {
		t.Errorf("expected openai format for kimi apikey path, got %q", tr.Format)
	}
}

func TestBuildTransportURLSuffix(t *testing.T) {
	tr := Transport{BaseURL: "https://api.kimi.com/coding/v1/messages", Format: "anthropic", URLSuffix: "?beta=true"}
	got := BuildTransportURL(tr, "kimi-k3", false)
	want := "https://api.kimi.com/coding/v1/messages?beta=true"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestBuildTransportURLGemini(t *testing.T) {
	tr := Transport{BaseURL: "https://generativelanguage.googleapis.com/v1beta/models", Format: "gemini"}
	got := BuildTransportURL(tr, "gemini-2.0-flash", true)
	want := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:streamGenerateContent?alt=sse"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// Phase 31: Gemini API keys go in the x-goog-api-key header (raw scheme), not
// the ?key= query param. The registry entry must declare an explicit auth that
// overrides the format-derived query default.
func TestResolveTransportGeminiAPIKeyHeader(t *testing.T) {
	p := Registry["gemini"]
	conn := &model.ProviderConnection{
		Provider: "gemini", AuthType: "api-key",
		Data: model.ConnectionData{APIKey: "AIza-test"},
	}
	tr := ResolveTransport(p, p.BaseURL, p.APIType, conn)

	if tr.Auth.Scheme != AuthRaw {
		t.Errorf("expected raw scheme for gemini api key, got %q", tr.Auth.Scheme)
	}
	if tr.Auth.Header != "x-goog-api-key" {
		t.Errorf("expected x-goog-api-key header, got %q", tr.Auth.Header)
	}
}

// Phase 31: GLM/MiniMax use the claude /coding endpoint with ?beta=true suffix,
// x-api-key raw auth, and CLAUDE_API_HEADERS — ported from 9router transport.
func TestResolveTransportClaudeCodingProviders(t *testing.T) {
	for _, id := range []string{"glm", "minimax", "minimax-cn"} {
		t.Run(id, func(t *testing.T) {
			p := Registry[id]
			conn := &model.ProviderConnection{
				Provider: id, AuthType: "api-key",
				Data: model.ConnectionData{APIKey: "sk-test"},
			}
			tr := ResolveTransport(p, p.BaseURL, p.APIType, conn)

			if tr.Auth.Scheme != AuthRaw {
				t.Errorf("expected raw scheme, got %q", tr.Auth.Scheme)
			}
			if tr.Auth.Header != "x-api-key" {
				t.Errorf("expected x-api-key header, got %q", tr.Auth.Header)
			}
			if tr.URLSuffix != "?beta=true" {
				t.Errorf("expected ?beta=true suffix, got %q", tr.URLSuffix)
			}
			if tr.Headers["anthropic-version"] != "2023-06-01" {
				t.Errorf("expected anthropic-version header, got %q", tr.Headers["anthropic-version"])
			}
			// URL building must append the suffix verbatim.
			got := BuildTransportURL(tr, "claude-3-5-sonnet", false)
			want := p.BaseURL + "?beta=true"
			if got != want {
				t.Errorf("expected %q, got %q", want, got)
			}
		})
	}
}

func TestBuildTransportURLOpenAIFullEndpoint(t *testing.T) {
	tr := Transport{BaseURL: "https://api.openai.com/v1/chat/completions", Format: "openai"}
	got := BuildTransportURL(tr, "gpt-4", false)
	want := "https://api.openai.com/v1/chat/completions"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestApplyAuthFormats validates header injection across 5+ auth shapes using a
// mock upstream that echoes the received headers.
func TestApplyAuthFormats(t *testing.T) {
	cases := []struct {
		creds Credentials
		check func(*testing.T, *http.Request)
		name  string
		tr    Transport
	}{
		{
			name:  "bearer",
			tr:    Transport{Auth: AuthDescriptor{Header: "Authorization", Scheme: AuthBearer}},
			creds: Credentials{APIKey: "sk-abc"},
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer sk-abc" {
					t.Errorf("expected Bearer sk-abc, got %q", got)
				}
			},
		},
		{
			name:  "raw x-api-key",
			tr:    Transport{Auth: AuthDescriptor{Header: "x-api-key", Scheme: AuthRaw, AnthropicVersion: true}},
			creds: Credentials{APIKey: "sk-ant-abc"},
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("x-api-key"); got != "sk-ant-abc" {
					t.Errorf("expected sk-ant-abc, got %q", got)
				}
				if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
					t.Errorf("expected anthropic-version injected, got %q", got)
				}
			},
		},
		{
			name:  "query key",
			tr:    Transport{Auth: AuthDescriptor{Scheme: AuthQuery, QueryParam: "key"}},
			creds: Credentials{APIKey: "AIza-abc"},
			check: func(t *testing.T, r *http.Request) {
				if got := r.URL.Query().Get("key"); got != "AIza-abc" {
					t.Errorf("expected key=AIza-abc, got %q", got)
				}
			},
		},
		{
			name:  "oauth access token bearer",
			tr:    Transport{Auth: AuthDescriptor{Header: "Authorization", Scheme: AuthBearer}},
			creds: Credentials{AccessToken: "oauth-tok"},
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer oauth-tok" {
					t.Errorf("expected Bearer oauth-tok, got %q", got)
				}
			},
		},
		{
			name:  "kimi headers hook + raw",
			tr:    Transport{Auth: AuthDescriptor{Header: "x-api-key", Scheme: AuthRaw, Hooks: []string{"kimiHeaders"}}},
			creds: Credentials{AccessToken: "kimi-tok", ProviderSpecificData: map[string]any{"deviceId": "dev-xyz"}},
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("x-api-key"); got != "kimi-tok" {
					t.Errorf("expected x-api-key=kimi-tok, got %q", got)
				}
				if got := r.Header.Get("X-Msh-Device-Id"); got != "dev-xyz" {
					t.Errorf("expected X-Msh-Device-Id=dev-xyz, got %q", got)
				}
				if got := r.Header.Get("X-Msh-Platform"); got == "" {
					t.Error("expected X-Msh-Platform header to be set")
				}
			},
		},
		{
			name:  "custom bearer header",
			tr:    Transport{Auth: AuthDescriptor{Header: "X-Custom-Auth", Scheme: AuthBearer}},
			creds: Credentials{APIKey: "tok-123"},
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("X-Custom-Auth"); got != "Bearer tok-123" {
					t.Errorf("expected Bearer tok-123 in custom header, got %q", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *http.Request
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Clone(r.Context())
				captured.URL = r.URL
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			req, _ := http.NewRequest("POST", srv.URL+"/test", nil)
			ApplyAuth(req, tc.tr, tc.creds)

			// Execute so query params are materialized on the wire.
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()

			if captured == nil {
				t.Fatal("upstream did not capture request")
			}
			tc.check(t, captured)
		})
	}
}

func TestApplyAuthNoToken(t *testing.T) {
	req, _ := http.NewRequest("POST", "https://example.com", nil)
	tr := Transport{Auth: AuthDescriptor{Header: "Authorization", Scheme: AuthBearer}}
	ApplyAuth(req, tr, Credentials{})
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("expected no auth header with empty token, got %q", got)
	}
}
