package db

import (
	"testing"
)

func TestBackfillRequestDetails_SingleConnectionNoDeadlock(t *testing.T) {
	d := setupTestDB(t)

	// Insert several usageHistory records
	for i := 1; i <= 5; i++ {
		entry := &UsageEntry{
			Timestamp:        "2026-09-10T12:00:00Z",
			Provider:         "test-provider",
			Model:            "test-model",
			ConnectionID:     "conn-1",
			APIKey:           "sk-test",
			Endpoint:         "/v1/chat/completions",
			PromptTokens:     10 * i,
			CompletionTokens: 20 * i,
			Status:           "ok",
		}
		if err := d.SaveUsageEntry(entry); err != nil {
			t.Fatalf("SaveUsageEntry %d: %v", i, err)
		}
	}

	// Clear requestDetails table so backfill triggers
	if _, err := d.conn.Exec(`DELETE FROM requestDetails`); err != nil {
		t.Fatalf("clear requestDetails: %v", err)
	}

	// This must not deadlock on SetMaxOpenConns(1)
	d.BackfillRequestDetails()

	res, err := d.GetRequestDetails(RequestDetailFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("GetRequestDetails: %v", err)
	}
	if res.Pagination.TotalItems != 5 {
		t.Errorf("expected 5 backfilled items, got %d", res.Pagination.TotalItems)
	}
}
