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

func TestSaveUsageEntryEveryCallIsNewEvent(t *testing.T) {
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
	// Each call records a new request, even for the same object or a foreign ID.
	entry.ID = 1
	if err := d.SaveUsageEntry(entry); err != nil {
		t.Fatal(err)
	}

	entries, err := d.GetUsageHistory(UsageFilter{Limit: 10})
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(entries), err)
	}
	day, err := d.GetUsageDaily("2026-07-22")
	if err != nil || day.Requests != 2 {
		t.Fatalf("expected daily 2, got %+v: %v", day, err)
	}
	lifetime, err := d.GetTotalRequestsLifetime()
	if err != nil || lifetime != 2 {
		t.Fatalf("expected lifetime 2, got %d: %v", lifetime, err)
	}
}

func TestSaveUsageEntrySameSecondDistinctDimensions(t *testing.T) {
	d := setupTestDB(t)
	for _, variant := range []struct{ key, endpoint, status string }{
		{"key-a", "/v1/chat/completions", "ok"},
		{"key-b", "/v1/chat/completions", "ok"},
		{"key-a", "/v1/messages", "ok"},
		{"key-a", "/v1/chat/completions", "error"},
	} {
		if err := d.SaveUsageEntry(&UsageEntry{
			Timestamp: "2026-07-22T10:00:00Z", Provider: "openai", Model: "m", ConnectionID: "conn",
			APIKey: variant.key, Endpoint: variant.endpoint, Status: variant.status,
			PromptTokens: 10, CompletionTokens: 5, Cost: 0.25,
		}); err != nil {
			t.Fatal(err)
		}
	}
	history, err := d.GetUsageHistory(UsageFilter{})
	if err != nil || len(history) != 4 {
		t.Fatalf("history = %d, want 4: %v", len(history), err)
	}
	if history[0].Status != "error" {
		t.Fatalf("lost error event: %+v", history[0])
	}
	day, err := d.GetUsageDaily("2026-07-22")
	if err != nil || day.Requests != 4 || day.PromptTokens != 40 || day.CompletionTokens != 20 || day.Cost != 1 || day.ByProvider["openai"].Requests != 4 || day.ByModel["m|openai"].Requests != 4 {
		t.Fatalf("daily = %+v: %v", day, err)
	}
	lifetime, err := d.GetTotalRequestsLifetime()
	if err != nil || lifetime != 4 {
		t.Fatalf("lifetime = %d, want 4: %v", lifetime, err)
	}
	keys, endpoints, err := d.GetUsageDimensions("")
	if err != nil || keys["key-a"].Requests != 3 || keys["key-b"].Requests != 1 || endpoints["/v1/chat/completions"].Requests != 3 || endpoints["/v1/messages"].Requests != 1 {
		t.Fatalf("dimensions = %+v / %+v: %v", keys, endpoints, err)
	}
}

func TestSaveUsageEntryDailyUsesUTC(t *testing.T) {
	d := setupTestDB(t)
	if err := d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-22T01:00:00+02:00", Provider: "p", Model: "m", PromptTokens: 10,
	}); err != nil {
		t.Fatal(err)
	}
	day, err := d.GetUsageDaily("2026-07-21")
	if err != nil || day.Requests != 1 || day.PromptTokens != 10 {
		t.Fatalf("UTC daily bucket not used: %+v: %v", day, err)
	}
}

func TestGetUsageDimensionsPropagatesQueryErrors(t *testing.T) {
	d := setupTestDB(t)
	if _, err := d.conn.Exec(`DROP TABLE usageHistory`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.GetUsageDimensions(""); err == nil {
		t.Fatal("query failure was reported as empty usage")
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

func TestGetUsageDimensions(t *testing.T) {
	d := setupTestDB(t)

	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-20T10:00:00Z", Provider: "openai", Model: "gpt-4o",
		APIKey: "sk-client-a", Endpoint: "/v1/chat/completions",
		PromptTokens: 10, CompletionTokens: 5, Cost: 0.01,
	})
	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-22T10:00:00Z", Provider: "anthropic", Model: "claude-3-5-sonnet",
		APIKey: "sk-client-b", Endpoint: "/v1/messages",
		PromptTokens: 30, CompletionTokens: 15, Cost: 0.05,
	})
	d.SaveUsageEntry(&UsageEntry{
		Timestamp: "2026-07-22T11:00:00Z", Provider: "qoder", Model: "efficient",
		APIKey: "", Endpoint: "/v1/chat/completions",
		PromptTokens: 20, CompletionTokens: 10, Cost: 0.0,
	})

	byKey, byEndpoint, err := d.GetUsageDimensions("2026-07-21T00:00:00Z")
	if err != nil {
		t.Fatalf("GetUsageDimensions: %v", err)
	}

	if len(byKey) != 2 {
		t.Fatalf("expected 2 keys (sk-client-b and default), got %d", len(byKey))
	}
	if byKey["sk-client-b"].Requests != 1 || byKey["sk-client-b"].PromptTokens != 30 {
		t.Errorf("unexpected stats for sk-client-b: %+v", byKey["sk-client-b"])
	}
	if byKey["default"].Requests != 1 || byKey["default"].PromptTokens != 20 {
		t.Errorf("unexpected stats for default key: %+v", byKey["default"])
	}

	if len(byEndpoint) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(byEndpoint))
	}
	if byEndpoint["/v1/messages"].Requests != 1 {
		t.Errorf("unexpected stats for /v1/messages: %+v", byEndpoint["/v1/messages"])
	}
	if byEndpoint["/v1/chat/completions"].Requests != 1 {
		t.Errorf("unexpected stats for /v1/chat/completions: %+v", byEndpoint["/v1/chat/completions"])
	}
}
