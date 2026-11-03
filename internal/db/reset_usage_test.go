package db

import (
	"testing"
	"time"
)

// Reset must clear every derived store, not just the raw history: leaving
// usageDaily or the lifetime counter behind would keep reporting totals for
// rows that no longer exist.
func TestResetUsageClearsAllDerivedStores(t *testing.T) {
	d := setupTestDB(t)

	if err := d.SaveUsageEntry(&UsageEntry{
		Provider: "openai", Model: "gpt-4o", Status: "ok",
		PromptTokens: 100, CompletionTokens: 10, Cost: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveRequestDetail(&RequestDetail{
		Provider: "openai", Model: "gpt-4o", Status: "ok", Data: "{}",
	}); err != nil {
		t.Fatal(err)
	}
	if lifetime, _ := d.GetTotalRequestsLifetime(); lifetime != 1 {
		t.Fatalf("lifetime before reset = %d, want 1", lifetime)
	}

	history, details, err := d.ResetUsage()
	if err != nil {
		t.Fatal(err)
	}
	if history != 1 || details != 1 {
		t.Fatalf("reset deleted history=%d details=%d, want 1/1", history, details)
	}

	if lifetime, _ := d.GetTotalRequestsLifetime(); lifetime != 0 {
		t.Errorf("lifetime after reset = %d, want 0", lifetime)
	}
	summary, err := d.GetUsageSummary(time.Time{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Requests != 0 {
		t.Errorf("summary still reports %d requests", summary.Requests)
	}
	if len(summary.ByProvider) != 0 {
		t.Errorf("byProvider not cleared: %+v", summary.ByProvider)
	}
	if res, err := d.GetRequestDetails(RequestDetailFilter{PageSize: 10}); err != nil || len(res.Details) != 0 {
		t.Errorf("request details survived reset: err=%v n=%d", err, len(res.Details))
	}

	// Idempotent: a second reset on an empty database is a no-op, not an error.
	if _, _, err := d.ResetUsage(); err != nil {
		t.Errorf("second reset should be a no-op, got %v", err)
	}
}
