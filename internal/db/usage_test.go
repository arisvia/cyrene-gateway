package db

import (
	"os"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "cyrene-test-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	d, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSaveUsageEntry(t *testing.T) {
	d := setupTestDB(t)

	entry := &UsageEntry{
		Timestamp:        "2026-07-22T10:00:00Z",
		Provider:         "openai",
		Model:            "gpt-4o",
		ConnectionID:     "conn-1",
		APIKey:           "sk-test123",
		Endpoint:         "/v1/chat/completions",
		PromptTokens:     100,
		CompletionTokens: 50,
		Status:           "ok",
	}

	if err := d.SaveUsageEntry(entry); err != nil {
		t.Fatalf("SaveUsageEntry: %v", err)
	}

	// Verify history
	entries, err := d.GetUsageHistory(UsageFilter{Limit: 10})
	if err != nil {
		t.Fatalf("GetUsageHistory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Provider != "openai" {
		t.Errorf("expected provider openai, got %s", entries[0].Provider)
	}
	if entries[0].PromptTokens != 100 {
		t.Errorf("expected 100 prompt tokens, got %d", entries[0].PromptTokens)
	}

	// Verify daily aggregation
	day, err := d.GetUsageDaily("2026-07-22")
	if err != nil {
		t.Fatalf("GetUsageDaily: %v", err)
	}
	if day.Requests != 1 {
		t.Errorf("expected 1 request, got %d", day.Requests)
	}
	if day.PromptTokens != 100 {
		t.Errorf("expected 100 prompt tokens in daily, got %d", day.PromptTokens)
	}
	if day.ByProvider["openai"].Requests != 1 {
		t.Errorf("expected 1 request for openai in byProvider")
	}

	// Verify lifetime counter
	lifetime, err := d.GetTotalRequestsLifetime()
	if err != nil {
		t.Fatalf("GetTotalRequestsLifetime: %v", err)
	}
	if lifetime != 1 {
		t.Errorf("expected lifetime 1, got %d", lifetime)
	}
}

func TestSaveUsageEntryDedup(t *testing.T) {
	d := setupTestDB(t)

	entry := &UsageEntry{
		Timestamp:        "2026-07-22T10:00:00Z",
		Provider:         "openai",
		Model:            "gpt-4o",
		ConnectionID:     "conn-1",
		PromptTokens:     100,
		CompletionTokens: 50,
	}

	if err := d.SaveUsageEntry(entry); err != nil {
		t.Fatal(err)
	}
	// Save same entry again — should be deduped
	if err := d.SaveUsageEntry(entry); err != nil {
		t.Fatal(err)
	}

	entries, _ := d.GetUsageHistory(UsageFilter{Limit: 10})
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after dedup, got %d", len(entries))
	}
}

func TestGetUsageHistoryFilter(t *testing.T) {
	d := setupTestDB(t)

	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-22T10:00:00Z", Provider: "openai", Model: "gpt-4o",
		PromptTokens: 10, CompletionTokens: 5,
	})
	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-22T11:00:00Z", Provider: "anthropic", Model: "claude-3",
		PromptTokens: 20, CompletionTokens: 10,
	})

	// Filter by provider
	entries, _ := d.GetUsageHistory(UsageFilter{Provider: "openai", Limit: 10})
	if len(entries) != 1 {
		t.Fatalf("expected 1 openai entry, got %d", len(entries))
	}

	// Filter by model
	entries, _ = d.GetUsageHistory(UsageFilter{Model: "claude-3", Limit: 10})
	if len(entries) != 1 {
		t.Fatalf("expected 1 claude-3 entry, got %d", len(entries))
	}
}

func TestSaveRequestDetail(t *testing.T) {
	d := setupTestDB(t)

	rd := &RequestDetail{
		ID:           "detail-1",
		Timestamp:    "2026-07-22T10:00:00Z",
		Provider:     "openai",
		Model:        "gpt-4o",
		ConnectionID: "conn-1",
		Status:       "ok",
		Data:         `{"id":"detail-1","provider":"openai","model":"gpt-4o","latency":{"total":150}}`,
	}

	if err := d.SaveRequestDetail(rd); err != nil {
		t.Fatalf("SaveRequestDetail: %v", err)
	}

	// Get by ID
	detail, err := d.GetRequestDetailByID("detail-1")
	if err != nil {
		t.Fatalf("GetRequestDetailByID: %v", err)
	}
	if detail == nil {
		t.Fatal("expected detail, got nil")
	}

	// Get paginated
	result, err := d.GetRequestDetails(RequestDetailFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("GetRequestDetails: %v", err)
	}
	if result.Pagination.TotalItems != 1 {
		t.Errorf("expected 1 total item, got %d", result.Pagination.TotalItems)
	}
	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d", len(result.Details))
	}
}

func TestGetUsageDailyRange(t *testing.T) {
	d := setupTestDB(t)

	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-20T10:00:00Z", Provider: "openai", Model: "gpt-4o",
		PromptTokens: 10, CompletionTokens: 5,
	})
	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-22T10:00:00Z", Provider: "openai", Model: "gpt-4o",
		PromptTokens: 20, CompletionTokens: 10,
	})

	days, err := d.GetUsageDailyRange("2026-07-21")
	if err != nil {
		t.Fatalf("GetUsageDailyRange: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("expected 1 day from 2026-07-21 onwards, got %d", len(days))
	}
	if _, ok := days["2026-07-22"]; !ok {
		t.Error("expected 2026-07-22 in range")
	}
}
