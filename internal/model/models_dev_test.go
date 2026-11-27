package model

import (
	"testing"
	"time"
)

// TestModelsDevCacheCodecKeepsFetchStamp: the persisted catalog must remember when
// it was fetched. Loading it stamped "now" reset the 24h TTL on every boot, so an
// installed gateway never fetched models.dev again and left `created` empty forever.
func TestModelsDevCacheCodecKeepsFetchStamp(t *testing.T) {
	stamp := time.Date(2026, time.November, 1, 8, 0, 0, 0, time.UTC)
	raw, err := EncodeModelsDevCache(map[string]ModelsDevEntry{
		"gpt-4o": {Name: "GPT-4o", Context: 128000, Output: 16384, ReleaseDate: "2024-05-13"},
	}, stamp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	entries, got, err := DecodeModelsDevCache(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Equal(stamp) {
		t.Fatalf("fetch stamp = %v, want %v", got, stamp)
	}
	if entries["gpt-4o"].ReleaseDate != "2024-05-13" {
		t.Fatalf("entry lost its release date: %+v", entries["gpt-4o"])
	}

	// Bare entry map without the envelope is rejected so unrecognized/outdated layouts trigger a fresh fetch.
	legacy := `{"gpt-4o":{"name":"OpenAI GPT-4o","family":"gpt","context":128000,"output":16384,"release_date":""}}`
	if _, _, err := DecodeModelsDevCache(legacy); err == nil {
		t.Errorf("bare map payload without fetched_at envelope must be rejected")
	}

	if _, _, err := DecodeModelsDevCache(`{"entries":{"gpt-4o":{}}}`); err == nil {
		t.Errorf("envelope without fetched_at must be rejected, not treated as a model named fetched_at")
	}
	if _, _, err := DecodeModelsDevCache(`[1,2,3]`); err == nil {
		t.Errorf("garbage payload must be an error")
	}
}

// The cap is only trusted from an exact identifier match: a fuzzy hit would quietly
// shorten answers for a sibling model that really allows a longer output, and unlike a
// rejected request that failure is invisible to the caller.
func TestModelsDevOutputLimit(t *testing.T) {
	seedModelsDev(t, map[string]ModelsDevEntry{
		"kimi-k2":         {Name: "Kimi K2", Output: 32768},
		"glm-4.6":         {Name: "GLM-4.6", Output: 16384},
		"no-limit-yet":    {Name: "No Limit Yet"},
		"deep-seek-model": {Name: "Deep Seek Model", Output: 8192},
	})

	if got := ModelsDevOutputLimit("volcengine-ark", "kimi-k2"); got != 32768 {
		t.Errorf("provider/id lookup = %d, want 32768", got)
	}
	if got := ModelsDevOutputLimit("", "glm-4.6"); got != 16384 {
		t.Errorf("bare id lookup = %d, want 16384", got)
	}
	if got := ModelsDevOutputLimit("x", "deep seek model"); got != 8192 {
		t.Errorf("spaced id lookup = %d, want 8192", got)
	}
	if got := ModelsDevOutputLimit("volcengine-ark", "kimi-k2-instruct-quantized-4bit"); got != 0 {
		t.Errorf("a longer id that merely contains a catalog key must not cap traffic, got %d", got)
	}
	if got := ModelsDevOutputLimit("p", "no-limit-yet"); got != 0 {
		t.Errorf("entry without an output limit = %d, want 0", got)
	}
	if got := ModelsDevOutputLimit("p", "absent-model"); got != 0 {
		t.Errorf("unknown model = %d, want 0", got)
	}
}
