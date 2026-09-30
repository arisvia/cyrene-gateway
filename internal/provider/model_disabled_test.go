package provider

import (
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// The disabled list is keyed by canonical id while a client may pin an upstream
// with a shorthand, so matching only the typed spelling lets the shorthand reach
// an upstream the operator switched off.
func TestIsModelDisabledMatchesShorthandPrefix(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	if err := database.KVSet(model.KVScopeDisabledModels, "openai/gpt-4o", "true"); err != nil {
		t.Fatalf("seed disabled model: %v", err)
	}

	cases := []struct {
		modelStr string
		want     bool
	}{
		{"openai/gpt-4o", true},
		{"oai/gpt-4o", true},
		{"gpt-4o", true},
		{"openai/gpt-4o-mini", false},
		{"oai/gpt-4o-mini", false},
		{"gpt-4o-mini", false},
	}
	for _, tc := range cases {
		if got := IsModelDisabled(tc.modelStr, database); got != tc.want {
			t.Errorf("IsModelDisabled(%q) = %v, want %v", tc.modelStr, got, tc.want)
		}
	}
}
