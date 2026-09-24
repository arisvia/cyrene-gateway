package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/auth"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestHandleRegistryEnrichedCapabilities(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Test GET /api/registry (grouped by categories)
	req := newLocalRequest("GET", "/api/registry", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Categories []struct {
			Category  string `json:"category"`
			Count     int    `json:"count"`
			Providers []struct {
				ID           string   `json:"id"`
				Name         string   `json:"name"`
				Capabilities []string `json:"capabilities"`
			} `json:"providers"`
		} `json:"categories"`
		Total int `json:"total"`
	}

	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal registry response: %v", err)
	}

	if resp.Total == 0 || len(resp.Categories) == 0 {
		t.Fatalf("expected non-empty categories in registry response")
	}

	// Verify that 'media' category is present in the response
	foundMediaCategory := false
	for _, cat := range resp.Categories {
		if cat.Category == "media" {
			foundMediaCategory = true
			if len(cat.Providers) == 0 {
				t.Errorf("media category should contain synthesized pure media providers")
			}
			for _, p := range cat.Providers {
				if len(p.Capabilities) == 0 {
					t.Errorf("pure media provider %q has empty capabilities", p.ID)
				}
				for _, cap := range p.Capabilities {
					if cap == "llm" {
						t.Errorf("pure media provider %q should not have llm capability", p.ID)
					}
				}
			}
		}
	}
	if !foundMediaCategory {
		t.Errorf("expected 'media' category to be synthesized in /api/registry")
	}

	// Verify openai has both llm and media capabilities (image, tts, stt, embedding)
	var openaiCaps []string
	for _, cat := range resp.Categories {
		for _, p := range cat.Providers {
			if p.ID == "openai" {
				openaiCaps = p.Capabilities
				break
			}
		}
	}
	if len(openaiCaps) == 0 {
		t.Fatalf("openai provider not found in registry")
	}

	hasLLM := false
	hasImage := false
	hasEmbedding := false
	for _, c := range openaiCaps {
		if c == "llm" {
			hasLLM = true
		}
		if c == "image" {
			hasImage = true
		}
		if c == "embedding" {
			hasEmbedding = true
		}
	}
	if !hasLLM || !hasImage || !hasEmbedding {
		t.Errorf("openai missing expected capabilities: %v", openaiCaps)
	}

	// 2. Test GET /api/registry?category=media
	reqMedia := newLocalRequest("GET", "/api/registry?category=media", nil)
	wMedia := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wMedia, reqMedia)
	if wMedia.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wMedia.Code)
	}

	var mediaResp struct {
		Providers []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(wMedia.Body.Bytes(), &mediaResp); err != nil {
		t.Fatalf("failed to unmarshal media registry response: %v", err)
	}
	if mediaResp.Count == 0 {
		t.Errorf("expected providers in /api/registry?category=media")
	}
}

func TestHandleMediaProvidersConnectedFiltering(t *testing.T) {
	srv, database := setupTestServer(t)

	// Inactive / No connection initially
	req := newLocalRequest("GET", "/api/media-providers?connected=true", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var emptyResp struct {
		Providers []struct {
			Provider string `json:"provider"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &emptyResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if emptyResp.Count != 0 {
		t.Errorf("expected 0 connected providers, got %d", emptyResp.Count)
	}

	// Add an active connection for antigravity
	conn := &model.ProviderConnection{
		ID:       "test-antigravity-conn",
		Provider: "antigravity",
		AuthType: "oauth",
		IsActive: true,
		Data: model.ConnectionData{
			AccessToken: "test-token",
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatalf("failed to create connection: %v", err)
	}

	// Query again with connected=true
	req = newLocalRequest("GET", "/api/media-providers?connected=true&kind=image", nil)
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var connectedResp struct {
		Providers []struct {
			Provider          string `json:"provider"`
			HasConnection     bool   `json:"hasConnection"`
			ActiveConnections int    `json:"activeConnections"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &connectedResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if connectedResp.Count != 1 {
		t.Fatalf("expected exactly 1 connected provider, got %d", connectedResp.Count)
	}
	if connectedResp.Providers[0].Provider != "antigravity" {
		t.Errorf("expected antigravity, got %s", connectedResp.Providers[0].Provider)
	}
	if !connectedResp.Providers[0].HasConnection || connectedResp.Providers[0].ActiveConnections != 1 {
		t.Errorf("expected HasConnection=true and ActiveConnections=1")
	}

	// Query all providers (without connected=true) -> should return all registered image providers
	reqAll := newLocalRequest("GET", "/api/media-providers?kind=image", nil)
	wAll := httptest.NewRecorder()
	srv.Handler.ServeHTTP(wAll, reqAll)

	var allResp struct {
		Providers []struct {
			Provider      string `json:"provider"`
			HasConnection bool   `json:"hasConnection"`
		} `json:"providers"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(wAll.Body.Bytes(), &allResp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if allResp.Count <= 1 {
		t.Errorf("expected multiple image providers in all listing, got %d", allResp.Count)
	}
}

func TestAggregateAntigravitySearchResponse(t *testing.T) {
	srv, _ := setupTestServer(t)

	// 1. Test success response with Google response.candidates envelope
	mockRespJSON := `{
		"response": {
			"candidates": [
				{
					"content": {
						"parts": [
							{"text": "Noah Lyles won the 2024 Olympic 100m gold."}
						]
					},
					"groundingMetadata": {
						"webSearchQueries": ["2024 olympics 100m gold winner"],
						"groundingChunks": [
							{
								"web": {
									"uri": "https://en.wikipedia.org/wiki/Athletics_at_the_2024_Summer_Olympics",
									"title": "Wikipedia"
								}
							}
						]
					}
				}
			]
		}
	}`

	httpResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(mockRespJSON))),
	}

	w := httptest.NewRecorder()
	srv.aggregateAntigravitySearchResponse(w, httpResp, "who won 100m")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var out struct {
		Provider string `json:"provider"`
		Query    string `json:"query"`
		Count    int    `json:"count"`
		Summary  string `json:"summary"`
		Results  []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if out.Provider != "antigravity" || out.Query != "who won 100m" || out.Count != 1 {
		t.Errorf("unexpected output: %+v", out)
	}
	if out.Summary != "Noah Lyles won the 2024 Olympic 100m gold." {
		t.Errorf("unexpected summary: %q", out.Summary)
	}
	if len(out.Results) != 1 || out.Results[0].Title != "Wikipedia" {
		t.Errorf("unexpected results: %+v", out.Results)
	}

	// 2. Test error response propagation (e.g. 503 capacity exhausted)
	mockErrJSON := `{"error": {"code": 503, "message": "No capacity available for model gemini-2.5-flash", "status": "UNAVAILABLE"}}`
	errHttpResp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(mockErrJSON))),
	}
	wErr := httptest.NewRecorder()
	srv.aggregateAntigravitySearchResponse(wErr, errHttpResp, "test query")
	if wErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", wErr.Code)
	}
}

func TestHandleEmbeddingsProviderHintRouting(t *testing.T) {
	srv, database := setupTestServer(t)

	var receivedPath string
	var receivedBody map[string]any
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer mockUpstream.Close()

	// Setup an active openrouter connection pointing to mockUpstream with trailing /chat/completions
	conn := model.ProviderConnection{
		ID:       "test-openrouter-conn",
		Provider: "openrouter",
		AuthType: "api-key",
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:  "sk-test-key",
			BaseURL: mockUpstream.URL + "/chat/completions",
		},
	}
	database.CreateConnection(&conn)

	// Call handleEmbeddings with model="openai/text-embedding-3-small" and provider hint "openrouter"
	reqBody := `{"provider":"openrouter","model":"openai/text-embedding-3-small","input":"hello"}`
	req := httptest.NewRequest("POST", "/v1/embeddings", bytes.NewReader([]byte(reqBody)))
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if receivedPath != "/embeddings" {
		t.Errorf("expected trimmed path '/embeddings', got %q", receivedPath)
	}
	if receivedBody["model"] != "openai/text-embedding-3-small" {
		t.Errorf("expected preserved model 'openai/text-embedding-3-small', got %v", receivedBody["model"])
	}
	if _, leaked := receivedBody["provider"]; leaked {
		t.Errorf("provider hint leaked into upstream body")
	}
}

// Only replace the external HTTP transport: routing, authentication, model
// resolution, credentials, and request serialization all use production code.
type mediaTestTransport func(*http.Request) (*http.Response, error)

func (f mediaTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func setupMediaAuthTest(t *testing.T, allowed []string, inspect func(*http.Request)) (*Server, string) {
	t.Helper()
	srv, database := setupTestServer(t)
	settings, err := database.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.RequireAPIKey = true
	if err := database.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	key := auth.GenerateAPIKey()
	if err := database.CreateAPIKey(&model.APIKey{
		ID: "media-key", Key: key, IsActive: true, AllowedModels: allowed,
	}); err != nil {
		t.Fatal(err)
	}
	for _, providerID := range []string{"openai", "xai", "antigravity"} {
		if err := database.CreateConnection(&model.ProviderConnection{
			ID: "media-" + providerID, Provider: providerID, AuthType: "api-key", IsActive: true,
			Data: model.ConnectionData{APIKey: "test-only-upstream-key"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		inspect(r)
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{"text":"transcribed","data":[]}`)),
		}, nil
	})}
	return srv, key
}

func mediaTestRequest(t *testing.T, path, modelName, providerID, key string) *http.Request {
	t.Helper()
	var req *http.Request
	if path == "/v1/audio/transcriptions" {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		for name, value := range map[string]string{
			"model": modelName, "language": "zh", "prompt": " Keep punctuation. ",
			"response_format": "verbose_json", "temperature": "0.25",
		} {
			if err := form.WriteField(name, value); err != nil {
				t.Fatal(err)
			}
		}
		file, err := form.CreateFormFile("file", "speech.wav")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte{'R', 'I', 'F', 'F', 0, 255, '\r', '\n', 128}); err != nil {
			t.Fatal(err)
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		req = httptest.NewRequest(http.MethodPost, path, &body)
		req.Header.Set("Content-Type", form.FormDataContentType())
	} else if strings.HasPrefix(path, "/v1/videos/job-") {
		req = httptest.NewRequest(http.MethodGet, path+"?provider="+providerID, nil)
	} else {
		body, err := json.Marshal(map[string]string{
			"model": modelName, "provider": providerID, "prompt": "test image",
			"input": "test speech", "voice": "alloy", "query": "test search",
		})
		if err != nil {
			t.Fatal(err)
		}
		req = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return req
}

func TestMediaReviewRejectsAmbiguousRoutingFields(t *testing.T) {
	for _, endpoint := range []struct{ path, provider, allowed, denied string }{
		{"/v1/images/generations", "openai", "openai/dall-e-3", "openai/secret-image"},
		{"/v1/audio/speech", "openai", "openai/tts-1", "openai/secret-tts"},
		{"/v1/videos/generations", "xai", "m", "grok-2-video"},
	} {
		for _, field := range []string{"model", "provider"} {
			for _, reverse := range []bool{false, true} {
				t.Run(endpoint.path+"/"+field+map[bool]string{false: "/lower-first", true: "/upper-first"}[reverse], func(t *testing.T) {
					calls := 0
					srv, key := setupMediaAuthTest(t, []string{endpoint.allowed, "xai/m"}, func(*http.Request) { calls++ })
					first, second := `"model":"`+endpoint.denied+`"`, `"Model":"`+endpoint.allowed+`"`
					other := `"provider":"` + endpoint.provider + `"`
					if field == "provider" {
						first, second = `"provider":"openai"`, `"PROVIDER":"xai"`
						other = `"model":"` + endpoint.allowed + `"`
					}
					if reverse {
						first, second = second, first
					}
					body := `{` + other + `,` + first + `,` + second + `,"prompt":"test","input":"speech","voice":"alloy"}`
					req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer "+key)
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					srv.Handler.ServeHTTP(w, req)
					if w.Code != http.StatusBadRequest || calls != 0 {
						t.Errorf("ambiguous fields must be rejected before upstream: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
					}
				})
			}
		}
	}
}

func TestMediaReviewCanonicalModelGoldenPaths(t *testing.T) {
	for _, endpoint := range []struct{ path, provider, model, prefix string }{
		{"/v1/images/generations", "openai", "dall-e-3", "oai"},
		{"/v1/audio/speech", "openai", "tts-1", "oai"},
		{"/v1/videos/generations", "xai", "grok-2-video", "xai"},
	} {
		target := endpoint.provider + "/" + endpoint.model
		for _, requested := range []string{endpoint.model, target, endpoint.prefix + "/" + endpoint.model, "media-alias"} {
			for _, modelField := range []string{"model", "MODEL"} {
				t.Run(endpoint.path+"/"+requested+"/"+modelField, func(t *testing.T) {
					calls := 0
					srv, key := setupMediaAuthTest(t, []string{requested, target}, func(r *http.Request) {
						calls++
						var body map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						for name := range body {
							if (strings.EqualFold(name, "model") && name != "model") || (strings.EqualFold(name, "provider") && name != "provider") {
								t.Errorf("noncanonical routing field reached upstream: %s", name)
							}
						}
						var actual string
						if err := json.Unmarshal(body["model"], &actual); err != nil || actual != endpoint.model {
							t.Errorf("upstream model=%q, want authorized %q (err=%v)", actual, endpoint.model, err)
						}
						if string(body["seed"]) != "9007199254740993" {
							t.Errorf("unrelated field lost precision: %s", body["seed"])
						}
					})
					if err := srv.DB.KVSet(model.KVScopeAliases, "media-alias", target); err != nil {
						t.Fatal(err)
					}
					body := `{"` + modelField + `":"` + requested + `","PROVIDER":"` + endpoint.provider + `","prompt":"test","input":"speech","voice":"alloy","seed":9007199254740993}`
					req := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(body))
					req.Header.Set("Authorization", "Bearer "+key)
					w := httptest.NewRecorder()
					srv.Handler.ServeHTTP(w, req)
					if w.Code != http.StatusOK || calls != 1 {
						t.Errorf("golden route failed: status=%d calls=%d body=%s", w.Code, calls, w.Body.String())
					}
				})
			}
		}
	}
}

func TestMediaModelWhitelist(t *testing.T) {
	for _, endpoint := range []struct {
		name, path, target, provider string
	}{
		{"image", "/v1/images/generations", "openai/dall-e-3", ""},
		{"tts", "/v1/audio/speech", "openai/tts-1", ""},
		{"stt", "/v1/audio/transcriptions", "openai/whisper-1", ""},
		{"video", "/v1/videos/generations", "xai/grok-2-video", ""},
		{"video_explicit_provider", "/v1/videos/generations", "xai/grok-2-video", "xai"},
		{"search", "/v1/search", "antigravity/gemini-2.5-flash", "antigravity"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, request string
				allowed       []string
				want          int
			}{
				{"denied_request", endpoint.target, []string{"deepseek/*"}, http.StatusForbidden},
				{"allowed_alias_denied_target", "media-alias", []string{"media-alias"}, http.StatusForbidden},
				{"denied_alias_allowed_target", "media-alias", []string{endpoint.target}, http.StatusForbidden},
				{"allowed_target", endpoint.target, []string{endpoint.target}, http.StatusOK},
				{"allowed_alias_and_target", "media-alias", []string{"media-alias", endpoint.target}, http.StatusOK},
			} {
				t.Run(tc.name, func(t *testing.T) {
					calls := 0
					srv, key := setupMediaAuthTest(t, tc.allowed, func(r *http.Request) {
						calls++
						if tc.want == http.StatusOK && endpoint.name != "stt" {
							var body struct {
								Model string `json:"model"`
							}
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Fatal(err)
							}
							_, wantModel, _ := strings.Cut(endpoint.target, "/")
							if body.Model != wantModel {
								t.Errorf("want upstream model %q, got %q", wantModel, body.Model)
							}
						}
					})
					if err := srv.DB.KVSet(model.KVScopeAliases, "media-alias", endpoint.target); err != nil {
						t.Fatal(err)
					}
					w := httptest.NewRecorder()
					srv.Handler.ServeHTTP(w, mediaTestRequest(t, endpoint.path, tc.request, endpoint.provider, key))
					if w.Code != tc.want {
						t.Errorf("want status %d, got %d: %s", tc.want, w.Code, w.Body.String())
					}
					wantCalls := 0
					if tc.want == http.StatusOK {
						wantCalls = 1
					} else if !strings.Contains(w.Body.String(), "not allowed for this API key") {
						t.Errorf("expected model whitelist rejection, got %s", w.Body.String())
					}
					if calls != wantCalls {
						t.Errorf("want %d upstream requests, got %d", wantCalls, calls)
					}
				})
			}
		})
	}
}

func TestMediaProviderOnlyVideoWhitelist(t *testing.T) {
	for _, path := range []string{"/v1/videos/generations", "/v1/videos/job-test"} {
		for _, tc := range []struct {
			name    string
			allowed []string
			want    int
		}{
			{"restricted", []string{"xai/*"}, http.StatusForbidden},
			{"unrestricted", nil, http.StatusOK},
			{"wildcard", []string{"*"}, http.StatusOK},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				calls := 0
				srv, key := setupMediaAuthTest(t, tc.allowed, func(*http.Request) { calls++ })
				w := httptest.NewRecorder()
				srv.Handler.ServeHTTP(w, mediaTestRequest(t, path, "", "xai", key))
				if w.Code != tc.want {
					t.Errorf("want status %d, got %d: %s", tc.want, w.Code, w.Body.String())
				}
				if tc.want == http.StatusForbidden && calls != 0 || tc.want == http.StatusOK && calls != 1 {
					t.Errorf("unexpected upstream requests: %d", calls)
				}
			})
		}
	}
}

func TestMediaSTTResolvedModelPreservesUpload(t *testing.T) {
	for _, requested := range []string{"openai/whisper-1", "oai/whisper-1", "speech-alias"} {
		t.Run(requested, func(t *testing.T) {
			calls := 0
			srv, key := setupMediaAuthTest(t, []string{requested, "openai/whisper-1"}, func(r *http.Request) {
				calls++
				if err := r.ParseMultipartForm(32 << 20); err != nil {
					t.Fatal(err)
				}
				defer r.MultipartForm.RemoveAll()
				wantFields := map[string][]string{
					"model": {"whisper-1"}, "language": {"zh"}, "prompt": {" Keep punctuation. "},
					"response_format": {"verbose_json"}, "temperature": {"0.25"},
				}
				if !reflect.DeepEqual(r.MultipartForm.Value, wantFields) {
					t.Errorf("upstream fields: want %v, got %v", wantFields, r.MultipartForm.Value)
				}
				file, header, err := r.FormFile("file")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				got, err := io.ReadAll(file)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, []byte{'R', 'I', 'F', 'F', 0, 255, '\r', '\n', 128}) || header.Filename != "speech.wav" {
					t.Errorf("upload changed: filename=%q bytes=%v", header.Filename, got)
				}
			})
			if err := srv.DB.KVSet(model.KVScopeAliases, "speech-alias", "openai/whisper-1"); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, mediaTestRequest(t, "/v1/audio/transcriptions", requested, "", key))
			if w.Code != http.StatusOK || calls != 1 {
				t.Fatalf("want 200 and one upstream request, got %d and %d: %s", w.Code, calls, w.Body.String())
			}
		})
	}
}

func TestMediaResolvedDisabledModel(t *testing.T) {
	for _, tc := range []struct {
		name, path, target, provider string
	}{
		{"image", "/v1/images/generations", "openai/dall-e-3", ""},
		{"tts", "/v1/audio/speech", "openai/tts-1", ""},
		{"stt", "/v1/audio/transcriptions", "openai/whisper-1", ""},
		{"video", "/v1/videos/generations", "xai/grok-2-video", ""},
		{"video_explicit_provider", "/v1/videos/generations", "xai/grok-2-video", "xai"},
		{"search", "/v1/search", "antigravity/gemini-2.5-flash", "antigravity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv, key := setupMediaAuthTest(t, nil, func(*http.Request) { calls++ })
			if err := srv.DB.KVSet(model.KVScopeAliases, "disabled-alias", tc.target); err != nil {
				t.Fatal(err)
			}
			if err := srv.DB.KVSet(model.KVScopeDisabledModels, tc.target, "true"); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, mediaTestRequest(t, tc.path, "disabled-alias", tc.provider, key))
			if w.Code != http.StatusForbidden || calls != 0 || !strings.Contains(w.Body.String(), "model is disabled") {
				t.Errorf("want disabled 403 and zero upstream requests, got %d and %d: %s", w.Code, calls, w.Body.String())
			}
		})
	}
}

func TestMediaOpenModeWithoutKey(t *testing.T) {
	for _, tc := range []struct {
		path, requested, provider string
	}{
		{"/v1/images/generations", "openai/dall-e-3", ""},
		{"/v1/audio/speech", "openai/tts-1", ""},
		{"/v1/audio/transcriptions", "openai/whisper-1", ""},
		{"/v1/videos/generations", "", "xai"},
		{"/v1/videos/job-test", "", "xai"},
		{"/v1/search", "", "antigravity"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			calls := 0
			srv, _ := setupMediaAuthTest(t, []string{"deepseek/*"}, func(*http.Request) { calls++ })
			settings, err := srv.DB.GetSettings()
			if err != nil {
				t.Fatal(err)
			}
			settings.RequireAPIKey = false
			if err := srv.DB.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, mediaTestRequest(t, tc.path, tc.requested, tc.provider, ""))
			if w.Code != http.StatusOK || calls != 1 {
				t.Errorf("want anonymous 200 and one upstream request, got %d and %d: %s", w.Code, calls, w.Body.String())
			}
		})
	}
}

func TestMediaSearchDefaultModelWhitelist(t *testing.T) {
	for _, tc := range []struct {
		name     string
		allowed  []string
		disabled bool
		want     int
	}{
		{"denied", []string{"openai/*"}, false, http.StatusForbidden},
		{"allowed", []string{"antigravity/gemini-2.5-flash"}, false, http.StatusOK},
		{"disabled", nil, true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv, key := setupMediaAuthTest(t, tc.allowed, func(*http.Request) { calls++ })
			if tc.disabled {
				if err := srv.DB.KVSet(model.KVScopeDisabledModels, "antigravity/gemini-2.5-flash", "true"); err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, mediaTestRequest(t, "/v1/search", "", "antigravity", key))
			if w.Code != tc.want {
				t.Errorf("want status %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
			if tc.want == http.StatusForbidden && calls != 0 || tc.want == http.StatusOK && calls != 1 {
				t.Errorf("unexpected upstream requests: %d", calls)
			}
		})
	}
}

func TestMediaExplicitProviderCannotBypassWhitelist(t *testing.T) {
	for _, tc := range []struct {
		path, target, provider string
	}{
		{"/v1/videos/generations", "openai/grok-2-video", "xai"},
		{"/v1/videos/generations", "xai/", "xai"},
		{"/v1/search", "openai/gemini-2.5-flash", "antigravity"},
		{"/v1/search", "antigravity/", "antigravity"},
	} {
		t.Run(tc.path+"/"+tc.target, func(t *testing.T) {
			calls := 0
			srv, key := setupMediaAuthTest(t, []string{"media-alias", tc.target}, func(*http.Request) { calls++ })
			if err := srv.DB.KVSet(model.KVScopeAliases, "media-alias", tc.target); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			srv.Handler.ServeHTTP(w, mediaTestRequest(t, tc.path, "media-alias", tc.provider, key))
			if w.Code != http.StatusForbidden || calls != 0 {
				t.Errorf("want 403 and zero upstream requests, got %d and %d: %s", w.Code, calls, w.Body.String())
			}
		})
	}
}
