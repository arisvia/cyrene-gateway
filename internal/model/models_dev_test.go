package model

import "testing"

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
