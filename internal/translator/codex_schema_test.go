package translator

import (
	"testing"
)

func TestStripCodexUnsupportedPatterns(t *testing.T) {
	unicodePattern := `^(?!__.*__$)[^\p{Cc}\p{Cf}\p{Zl}\p{Zp}"\\./\[\]]{1,200}$`
	validPattern := `^[a-z][a-z0-9_-]{0,31}$`

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"artifact": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":    "string",
						"pattern": unicodePattern,
					},
					"slug": map[string]any{
						"type":    "string",
						"pattern": validPattern,
					},
				},
			},
			// A property whose key name is "pattern" must be treated as data, not schema keyword
			"pattern": map[string]any{
				"type": "string",
			},
		},
	}

	StripCodexUnsupportedPatterns(schema)

	props := schema["properties"].(map[string]any)
	artifact := props["artifact"].(map[string]any)["properties"].(map[string]any)

	// Unicode property pattern should be deleted
	nameProp := artifact["name"].(map[string]any)
	if _, ok := nameProp["pattern"]; ok {
		t.Errorf("expected unicode property pattern to be stripped from nameProp, got %v", nameProp["pattern"])
	}

	// Valid pattern should be preserved
	slugProp := artifact["slug"].(map[string]any)
	if pat, ok := slugProp["pattern"].(string); !ok || pat != validPattern {
		t.Errorf("expected valid pattern to be preserved, got %v", slugProp["pattern"])
	}

	// Property named "pattern" must not be deleted
	if _, ok := props["pattern"]; !ok {
		t.Errorf("property named 'pattern' was accidentally deleted")
	}
}
