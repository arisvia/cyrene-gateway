package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// openAIMockModels serves a standard /models payload and captures auth.
func openAIMockModels(t *testing.T, wantAuth func(r *http.Request) error) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantAuth != nil {
			if err := wantAuth(r); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model-a","object":"model"},{"id":"model-b","object":"model"}]}`))
	}))
}

func TestFetchModels_BearerAuth(t *testing.T) {
	srv := openAIMockModels(t, func(r *http.Request) error { return nil })
	defer srv.Close()

	models, err := FetchModels(srv.Client(), "openai", srv.URL+"/chat/completions", "sk-test", "", ModelsFetchConfig{Auth: "bearer"})
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 2 || models[0].ID != "model-a" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestFetchModels_AuthSchemes(t *testing.T) {
	cases := []struct {
		name  string
		cfg   ModelsFetchConfig
		token string
		check func(*testing.T, *http.Request)
	}{
		{
			name: "none (public)",
			cfg:  ModelsFetchConfig{Auth: "none"},
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "" {
					t.Errorf("expected no auth header, got %q", got)
				}
			},
		},
		{
			name:  "query key",
			cfg:   ModelsFetchConfig{Auth: "query"},
			token: "AIza-key",
			check: func(t *testing.T, r *http.Request) {
				if got := r.URL.Query().Get("key"); got != "AIza-key" {
					t.Errorf("expected key=AIza-key, got %q", got)
				}
			},
		},
		{
			name:  "raw x-api-key",
			cfg:   ModelsFetchConfig{Auth: "raw", AuthHeader: "x-api-key"},
			token: "sk-ant",
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("x-api-key"); got != "sk-ant" {
					t.Errorf("expected x-api-key=sk-ant, got %q", got)
				}
			},
		},
		{
			name:  "bearer default",
			cfg:   ModelsFetchConfig{},
			token: "sk-bearer",
			check: func(t *testing.T, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "Bearer sk-bearer" {
					t.Errorf("expected Bearer sk-bearer, got %q", got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.check(t, r)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"data":[]}`))
			}))
			defer srv.Close()

			cfg := tc.cfg
			if cfg.URL == "" {
				cfg.URL = srv.URL + "/models"
			}
			_, err := FetchModels(srv.Client(), "openai", "", tc.token, "", cfg)
			if err != nil {
				t.Fatalf("FetchModels failed: %v", err)
			}
		})
	}
}

func TestFetchModels_ExplicitURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/catalog" {
			t.Errorf("expected /custom/catalog, got %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"m1","object":"model"}]}`))
	}))
	defer srv.Close()

	models, err := FetchModels(srv.Client(), "openai", "https://ignored.example", "", "", ModelsFetchConfig{URL: srv.URL + "/custom/catalog", Auth: "none"})
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestModelsDevBackfill(t *testing.T) {
	catalog := map[string]modelsDevEntry{
		"gpt-4o":         {Name: "GPT-4o", Family: "gpt", Context: 128000, Output: 16384},
		"claude-fable-5": {Name: "Claude Fable 5", Family: "claude", Context: 200000, Output: 32768},
	}

	models := []ModelMetadata{
		{ID: "gpt-4o"},                             // exact match
		{ID: "openai/gpt-4o"},                      // prefixed
		{ID: "claude-fable-5", ContextLength: 999}, // existing value preserved
		{ID: "unknown-model"},                      // no match
	}
	BackfillFromModelsDev(models, catalog)

	if models[0].ContextLength != 128000 || models[0].MaxOutput != 16384 || models[0].DisplayName != "GPT-4o" {
		t.Errorf("exact match backfill failed: %+v", models[0])
	}
	if models[1].ContextLength != 128000 {
		t.Errorf("prefixed id backfill failed: %+v", models[1])
	}
	if models[2].ContextLength != 999 {
		t.Errorf("existing value was overwritten: %+v", models[2])
	}
	if models[2].MaxOutput != 32768 {
		t.Errorf("missing field not filled for claude: %+v", models[2])
	}
	if models[3].ContextLength != 0 {
		t.Errorf("unknown model should stay empty: %+v", models[3])
	}
}

func TestLoadModelsDevCatalog_Mock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"name": "OpenAI",
				"models": map[string]any{
					"gpt-4o": map[string]any{
						"id": "gpt-4o", "name": "GPT-4o", "family": "gpt",
						"limit": map[string]any{"context": 128000, "output": 16384},
					},
				},
			},
		})
	}))
	defer srv.Close()

	orig := ModelsDevURL
	ModelsDevURL = srv.URL
	defer func() { ModelsDevURL = orig }()

	// Reset cache state for isolation.
	modelsDevMu.Lock()
	modelsDevCache = nil
	modelsDevMu.Unlock()

	catalog, err := LoadModelsDevCatalog(srv.Client())
	if err != nil {
		t.Fatalf("LoadModelsDevCatalog failed: %v", err)
	}
	e, ok := catalog["gpt-4o"]
	if !ok {
		t.Fatalf("expected gpt-4o in catalog")
	}
	if e.Context != 128000 || e.Output != 16384 {
		t.Errorf("unexpected limits: %+v", e)
	}
}
