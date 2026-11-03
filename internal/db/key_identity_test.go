package db

import (
	"testing"
	"time"
)

// Usage must group on the key id, not the secret: keys get renamed, and the
// panel cannot label a row by a value the operator never chose.
func TestUsageGroupsByKeyIDAndCarriesName(t *testing.T) {
	d := setupTestDB(t)

	for _, meta := range []string{
		`{"key_id":"kid-1","key_name":"team-a"}`,
		`{"key_id":"kid-1","key_name":"team-a"}`,
		`{"key_id":"kid-2","key_name":"team-b"}`,
	} {
		if err := d.SaveUsageEntry(&UsageEntry{
			Provider: "openai", Model: "gpt-4o", Status: "ok",
			APIKey: "cg-secret-value", PromptTokens: 10, Meta: meta,
		}); err != nil {
			t.Fatal(err)
		}
	}

	s, err := d.GetUsageSummary(time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ByKey["kid-1"].Requests; got != 2 {
		t.Errorf("kid-1 requests = %d, want 2", got)
	}
	if got := s.ByKey["kid-2"].Requests; got != 1 {
		t.Errorf("kid-2 requests = %d, want 1", got)
	}
	if _, leaked := s.ByKey["cg-secret-value"]; leaked {
		t.Error("grouped by the secret instead of the key id")
	}
	if s.KeyNames["kid-1"] != "team-a" || s.KeyNames["kid-2"] != "team-b" {
		t.Errorf("names not carried: %+v", s.KeyNames)
	}
}

// Rows written before key identity existed must keep working: they group by
// whatever was stored, so upgrading does not split a key's history in two.
func TestUsageLegacyKeyRowsStillGroup(t *testing.T) {
	d := setupTestDB(t)

	if err := d.SaveUsageEntry(&UsageEntry{
		Provider: "openai", Model: "gpt-4o", Status: "ok",
		APIKey: "cg-legacy-secret", PromptTokens: 10, Meta: `{}`,
	}); err != nil {
		t.Fatal(err)
	}

	s, err := d.GetUsageSummary(time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ByKey["cg-legacy-secret"].Requests; got != 1 {
		t.Errorf("legacy row lost: %+v", s.ByKey)
	}
}
