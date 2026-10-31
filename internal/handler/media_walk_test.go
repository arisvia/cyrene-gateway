package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/media"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func TestNormalizeImageGemini(t *testing.T) {
	raw := []byte(`{"candidates":[{"content":{"parts":[{"text":"here you go"},{"inlineData":{"mimeType":"image/png","data":"aW1nMQ=="}}]}}]}`)
	out, err := media.NormalizeImage("gemini", raw)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Created int64 `json:"created"`
		Data    []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].B64 != "aW1nMQ==" || body.Created == 0 {
		t.Fatalf("unexpected normalized body: %s", out)
	}
}

func TestNormalizeImageGeminiSnakeCaseAndMulti(t *testing.T) {
	raw := []byte(`{"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/webp","data":"QQ=="}},{"inline_data":{"mime_type":"image/webp","data":"Qg=="}}]}}]}`)
	out, err := media.NormalizeImage("gemini", raw)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 2 || body.Data[1]["b64_json"] != "Qg==" {
		t.Fatalf("multi-image normalization broken: %s", out)
	}
}

func TestNormalizeImageGeminiNoImageParts(t *testing.T) {
	if _, err := media.NormalizeImage("gemini", []byte(`{"candidates":[{"content":{"parts":[{"text":"refused"}]}}]}`)); err == nil {
		t.Fatal("expected error for image-less gemini response")
	}
}

func TestNormalizeImageStability(t *testing.T) {
	out, err := media.NormalizeImage("stability", []byte(`{"image":"c3Ri","seed":1,"finish_reason":"SUCCESS"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"b64_json":"c3Ri"`) {
		t.Fatalf("stability normalization broken: %s", out)
	}
}

func TestNormalizeAudioGeminiTTS(t *testing.T) {
	audio := []byte("fake-lpcm-bytes")
	env := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/l16;codec=pcm;rate=24000","data":"` + base64.StdEncoding.EncodeToString(audio) + `"}}]}}]}`
	out, mime, err := media.NormalizeAudio("gemini-tts", []byte(env))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(audio) || !strings.HasPrefix(mime, "audio/") {
		t.Fatalf("audio normalization broken: %q %q", out, mime)
	}
}

func TestNormalizeSTTDeepgramAndGemini(t *testing.T) {
	out, err := media.NormalizeSTT("deepgram", []byte(`{"results":{"channels":[{"alternatives":[{"transcript":"hello world"}]}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"text":"hello world"}` {
		t.Fatalf("deepgram normalization broken: %s", out)
	}
	out, err = media.NormalizeSTT("gemini-stt", []byte(`{"candidates":[{"content":{"parts":[{"text":"hello "},{"text":"world"}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"text":"hello world"}` {
		t.Fatalf("gemini stt normalization broken: %s", out)
	}
}

// mediaWalkFixture builds a two-account equal-priority provider whose mock
// fails the FIRST attempt with firstStatus and succeeds afterwards, so the
// walk must consume exactly two attempts regardless of RR start position.
func mediaWalkFixture(t *testing.T, firstStatus int) (*Server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	srv, database := setupTestServer(t)
	var hits, fails atomic.Int32
	var failNext atomic.Bool
	failNext.Store(true)
	for _, id := range []string{"walk-key-a", "walk-key-b"} {
		if err := database.CreateConnection(&model.ProviderConnection{
			ID: id, Provider: "openai", AuthType: "api-key", IsActive: true,
			Data: model.ConnectionData{APIKey: id},
		}); err != nil {
			t.Fatal(err)
		}
	}
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		hits.Add(1)
		if failNext.CompareAndSwap(true, false) {
			fails.Add(1)
			return &http.Response{
				StatusCode: firstStatus,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"quota exceeded"}}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"created":1,"data":[{"b64_json":"b2s="}]}`)),
		}, nil
	})}
	return srv, &hits, &fails
}

func TestMediaImageFailsOverOnQuotaExhausted(t *testing.T) {
	srv, hits, fails := mediaWalkFixture(t, http.StatusTooManyRequests)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"openai/dall-e-3","prompt":"a cat"}`))
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 after failover, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"b64_json":"b2s="`) {
		t.Fatalf("second-account response not proxied: %s", w.Body.String())
	}
	if hits.Load() != 2 || fails.Load() != 1 {
		t.Fatalf("want exactly one retry across two accounts, hits=%d fails=%d", hits.Load(), fails.Load())
	}

	// Exactly the failed account must be cooled down; the successful one not.
	conns, err := srv.DB.ListConnectionsByProvider("openai")
	if err != nil {
		t.Fatal(err)
	}
	cooled := 0
	for _, c := range conns {
		if c.Data.RateLimitedUntil != "" {
			cooled++
		}
	}
	if cooled != 1 {
		t.Fatalf("want exactly one cooled account after failover, got %d", cooled)
	}
}

func TestMediaImageTerminal4xxNoFailover(t *testing.T) {
	srv, hits, _ := mediaWalkFixture(t, http.StatusBadRequest)
	// Override: the fixture's "quota exceeded" body would classify as
	// retryable (text keyword), so use a request-scoped error body instead.
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		hits.Add(1)
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"invalid prompt"}}`)),
		}, nil
	})}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"openai/dall-e-3","prompt":"a cat"}`))
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("request-scoped 4xx must be terminal, got %d", w.Code)
	}
	if hits.Load() != 1 {
		t.Fatalf("terminal 4xx must not walk the pool, hits=%d", hits.Load())
	}
}

func TestMediaImagePoolExhaustion(t *testing.T) {
	srv, _, _ := mediaWalkFixture(t, http.StatusTooManyRequests)
	// Make the second account fail too: swap transport to always 429.
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"quota"}}`)),
		}, nil
	})}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"openai/dall-e-3","prompt":"a cat"}`))
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("exhausted pool must surface last upstream status, got %d: %s", w.Code, w.Body.String())
	}
}

// TestMediaAntigravityQuotaParksPoolNotAccount proves the media surface shares
// the chat-side pool semantics: an exhausted Gemini quota (image generation)
// must lock only the gemini pool's models on the antigravity account, leaving
// the Claude/GPT pool — and the account overall — servable for chat traffic.
func TestMediaAntigravityQuotaParksPoolNotAccount(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "ag-media", Provider: "antigravity", AuthType: "oauth", IsActive: true,
		Data: model.ConnectionData{
			AccessToken:          "tok",
			ProviderSpecificData: map[string]any{"projectId": "proj-1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"quota exceeded","code":429}}`)),
		}, nil
	})}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"antigravity/gemini-3.1-flash-image","prompt":"a cat"}`))
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want terminal 429 after single-account pool exhaustion, got %d: %s", w.Code, w.Body.String())
	}
	conns, err := srv.DB.ListConnectionsByProvider("antigravity")
	if err != nil || len(conns) == 0 {
		t.Fatalf("connection missing: err=%v n=%d", err, len(conns))
	}
	psd := conns[0].Data.ProviderSpecificData
	if conns[0].Data.RateLimitedUntil != "" {
		t.Errorf("media quota burn must NOT park the whole account (claude chat would melt down): RateLimitedUntil=%s", conns[0].Data.RateLimitedUntil)
	}
	if lock, _ := psd["modelLock_gemini-3.1-flash-image"].(string); lock == "" {
		t.Fatalf("expected pool lock on the exhausted gemini engine, got %+v", psd)
	}
	if _, banded := psd["modelLock_claude-opus-5-5-high"]; banded {
		t.Errorf("claude pool must stay servable when only the gemini media pool is exhausted")
	}
	pool := []model.ProviderConnection{conns[0]}
	if c := provider.SelectCredential(pool, "claude-opus-5-5-high", nil); c == nil {
		t.Errorf("claude chat must still select this account while only gemini is locked")
	}
	if c := provider.SelectCredential(pool, "gemini-3.1-flash-image", nil); c != nil {
		t.Errorf("selector must skip the pool-locked media engine")
	}
}

func TestMediaTTSNormalizesGeminiAudio(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "tts-gem", Provider: "gemini", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "gem-key"},
	}); err != nil {
		t.Fatal(err)
	}
	pcm := []byte("lpcm-audio")
	env := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/l16;codec=pcm;rate=24000","data":"` + base64.StdEncoding.EncodeToString(pcm) + `"}}]}}]}`
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(env)),
		}, nil
	})}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"gemini/gemini-2.5-flash-preview-tts","input":"hi","voice":"Kore"}`))
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != string(pcm) {
		t.Fatalf("raw audio not extracted from envelope: %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "audio/") {
		t.Fatalf("audio content type missing: %q", ct)
	}
}

func TestVideoJobRegistryResolvesOwnerWithoutProviderHint(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "vid-xai", Provider: "xai", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "xai-key"},
	}); err != nil {
		t.Fatal(err)
	}
	var paths []string
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/generations") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"job-abc-123","status":"queued"}`)),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"job-abc-123","status":"completed"}`)),
		}, nil
	})}

	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(`{"model":"xai/grok-2-video","prompt":"a rocket"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("video creation failed: %d %s", w.Code, w.Body.String())
	}

	// Poll WITHOUT the provider query param: the registry must resolve it.
	w = httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/videos/job-abc-123", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("provider-less status poll failed: %d %s", w.Code, w.Body.String())
	}
	if len(paths) != 2 || !strings.Contains(paths[1], "job-abc-123") {
		t.Fatalf("status poll did not reach upstream job endpoint: %v", paths)
	}
}

func TestMediaUsageLandsInUsageHistory(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "img-openai", Provider: "openai", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "k"},
	}); err != nil {
		t.Fatal(err)
	}
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"created":1,"data":[{"b64_json":"b2s="}]}`)),
		}, nil
	})}

	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"openai/dall-e-3","prompt":"a cat"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("image request failed: %d %s", w.Code, w.Body.String())
	}

	rows, err := database.GetUsageHistory(db.UsageFilter{Model: "dall-e-3"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Provider != "openai" || rows[0].Status != "ok" {
		t.Fatalf("media call missing from usageHistory: %+v", rows)
	}
}

// Media callers may name the model as "provider/model" (the web-search
// default engine does); usage rows must store the bare id like chat does,
// otherwise the usage dashboard splits one model across two buckets.
func TestMediaUsageRecordsBareModel(t *testing.T) {
	srv, database := setupTestServer(t)
	r := httptest.NewRequest(http.MethodPost, "/v1/search", nil)
	srv.recordMediaUsage(r, "antigravity", "antigravity/gemini-3.8-flash-high", "conn-1", "/v1/search", http.StatusOK, 12)

	rows, err := database.GetUsageHistory(db.UsageFilter{Model: "gemini-3.8-flash-high"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Model != "gemini-3.8-flash-high" {
		t.Fatalf("expected one bare-model usage row, got %+v", rows)
	}
	details, err := database.GetRequestDetails(db.RequestDetailFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Details) != 1 {
		t.Fatalf("expected one request detail, got %d", len(details.Details))
	}
	var rd struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(details.Details[0], &rd); err != nil {
		t.Fatal(err)
	}
	if rd.Model != "gemini-3.8-flash-high" {
		t.Fatalf("request detail should carry the bare model id, got %q", rd.Model)
	}
}

func TestMediaEgressHonorsProxyPool(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "img-openai", Provider: "openai", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "k"},
	}); err != nil {
		t.Fatal(err)
	}
	// A proxy pool whose "proxy" is an httptest server: media egress must
	// arrive there (CONNECT/absolute-form or plain path both acceptable).
	var sawMedia atomic.Bool
	proxyUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawMedia.Store(true)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"created":1,"data":[{"b64_json":"b2s="}]}`))
	}))
	defer proxyUp.Close()
	srv.Proxies.UpdatePools([]model.ProxyPool{{
		ID: "pool-1", IsActive: true,
		Data: model.ProxyPoolData{ProxyURL: proxyUp.URL},
	}})
	// The direct client would answer differently; if media bypassed the pool
	// this transport would be hit instead.
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("media request bypassed the proxy pool")
		return nil, fmt.Errorf("bypassed")
	})}

	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"openai/dall-e-3","prompt":"a cat"}`)))
	// The fake proxy speaks plain HTTP, so an HTTPS target can only get as
	// far as the CONNECT/tunnel stage: reaching it at all proves egress
	// used the pool transport, and the direct mock staying silent proves
	// no bypass. A completed 200 is equally acceptable.
	if !sawMedia.Load() {
		t.Fatalf("media egress did not traverse proxy pool: code=%d body=%s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK && !(w.Code == http.StatusBadGateway && strings.Contains(w.Body.String(), "upstream request failed")) {
		t.Fatalf("unexpected outcome for pool-routed media call: code=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "bypassed") {
		t.Fatal("media request used the direct client instead of the proxy pool")
	}
}

func TestMediaSTTNormalizesGeminiTranscript(t *testing.T) {
	srv, database := setupTestServer(t)
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "stt-gem", Provider: "gemini", AuthType: "api-key", IsActive: true,
		Data: model.ConnectionData{APIKey: "gem-key"},
	}); err != nil {
		t.Fatal(err)
	}
	srv.MediaClient.HTTPClient = &http.Client{Transport: mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"hello world"}]}}]}`)),
		}, nil
	})}

	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, mediaTestRequest(t, "/v1/audio/transcriptions", "gemini/gemini-2.5-flash", "", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"text":"hello world"}` {
		t.Fatalf("transcript not normalized to OpenAI shape: %s", got)
	}
}
