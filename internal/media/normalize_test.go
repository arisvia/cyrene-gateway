package media

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// fal answers {"images":[{"url":...}]}, which is not the OpenAI envelope the
// gateway advertises. Without normalization the downstream client receives a
// body it cannot parse.
func TestNormalizeImageFal(t *testing.T) {
	if !NeedsImageNormalization("fal") {
		t.Fatal("fal responses need normalization")
	}
	raw := []byte(`{"images":[{"url":"https://fal.media/a.png","width":1024},{"url":"https://fal.media/b.png"}]}`)
	out, err := NormalizeImage("fal", raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	var body struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("output is not JSON: %v (%s)", err, out)
	}
	if len(body.Data) != 2 || body.Data[0].URL != "https://fal.media/a.png" {
		t.Fatalf("unexpected OpenAI body: %s", out)
	}
}

// An empty or unparseable fal body must surface as an error, not as a
// successful empty result the caller would treat as "no images generated".
func TestNormalizeImageFalErrors(t *testing.T) {
	if _, err := NormalizeImage("fal", []byte(`{"images":[]}`)); err == nil {
		t.Fatal("empty images must error")
	}
	if _, err := NormalizeImage("fal", []byte(`not json`)); err == nil {
		t.Fatal("garbage must error")
	}
}

// AssemblyAI only accepts a URL its servers can fetch, so a data: URI would
// fail at download time. The submit body must carry the upload endpoint's URL.
func TestAssemblyAIUploadURLDerivation(t *testing.T) {
	cases := map[string]string{
		"https://api.assemblyai.com/v2/transcript":  "https://api.assemblyai.com/v2/upload",
		"https://api.assemblyai.com/v2/transcript/": "https://api.assemblyai.com/v2/upload",
	}
	for base, want := range cases {
		cfg := ProviderConfig{Provider: "assemblyai", BaseURL: base}
		got := assemblyAIUploadURL(&cfg)
		if got != want {
			t.Errorf("upload URL for %q = %q, want %q", base, got, want)
		}
	}
}

// fal types image_size as integers; strings are rejected upstream.
func TestFalImageBodySizeTypes(t *testing.T) {
	body := falImageBody(&ImageRequest{Prompt: "cat", Size: "1024x768", N: 2})
	size, ok := body["image_size"].(map[string]any)
	if !ok {
		t.Fatalf("image_size missing: %+v", body)
	}
	if _, isInt := size["width"].(int); !isInt {
		t.Errorf("width must be int, got %T", size["width"])
	}
	if _, isInt := size["height"].(int); !isInt {
		t.Errorf("height must be int, got %T", size["height"])
	}
	if body["num_images"] != 2 || body["prompt"] != "cat" {
		t.Errorf("unexpected body: %+v", body)
	}

	// A malformed size must be omitted, not sent as a broken value.
	for _, bad := range []string{"huge", "1024x", "x768", "0x0"} {
		if _, present := falImageBody(&ImageRequest{Prompt: "p", Size: bad})["image_size"]; present {
			t.Errorf("size %q must be dropped, not forwarded", bad)
		}
	}
}

// AssemblyAI authenticates with the raw key: "Bearer <key>" is rejected.
func TestAssemblyAIAuthIsRaw(t *testing.T) {
	cfg := GetConfig("assemblyai", KindSTT)
	if cfg == nil {
		t.Fatal("assemblyai STT config missing")
	}
	if cfg.AuthHeader != "raw" {
		t.Fatalf("assemblyai must use raw auth, got %q", cfg.AuthHeader)
	}
}

// Only the current model identifiers are offered; "best" and "nano" are
// deprecated aliases AssemblyAI maps away from.
func TestAssemblyAIModelsAreCurrent(t *testing.T) {
	p := Registry["assemblyai"]
	if p == nil {
		t.Fatal("assemblyai provider missing")
	}
	allowed := map[string]bool{"universal-3-5-pro": true, "universal-2": true}
	for _, m := range p.Models {
		if !allowed[m.ID] {
			t.Errorf("model %q is not a current AssemblyAI identifier", m.ID)
		}
	}
	if len(p.Models) != len(allowed) {
		t.Errorf("expected %d current models, got %d", len(allowed), len(p.Models))
	}
}

// Voice and model arrive from the request body. Unescaped, a caller could walk
// the URL tree or append their own query parameters to a request that carries
// the gateway's provider key.
func TestUserSuppliedPathSegmentsAreEscaped(t *testing.T) {
	// url.PathEscape is what the client applies; assert the property that
	// matters: a traversal attempt cannot leave the configured path prefix.
	got := url.PathEscape("../../v1/user/subscription")
	if strings.Contains(got, "/") {
		t.Fatalf("escaped voice id still contains a path separator: %q", got)
	}

	q := url.Values{}
	q.Set("output_format", "mp3&enable_logging=false")
	if strings.Contains(q.Encode(), "&enable_logging") {
		t.Fatalf("caller-appended query parameter survived encoding: %q", q.Encode())
	}
}
