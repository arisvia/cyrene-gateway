package db

import (
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestSaveRequestDetail_TrimsTo200AndConcurrent(t *testing.T) {
	d := setupTestDB(t)

	// 1. Clear any table state
	if _, err := d.conn.Exec(`DELETE FROM requestDetails`); err != nil {
		t.Fatalf("clear requestDetails: %v", err)
	}

	// 2. Insert 220 records sequentially with ordered timestamps
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 220 {
		rd := &RequestDetail{
			ID:        fmt.Sprintf("rd-seq-%04d", i),
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
			Provider:  "openai",
			Model:     "gpt-4o",
			Status:    "ok",
			Data:      fmt.Sprintf(`{"index":%d}`, i),
		}
		if err := d.SaveRequestDetail(rd); err != nil {
			t.Fatalf("SaveRequestDetail %d: %v", i, err)
		}
	}

	// Confirm table count is exactly defaultMaxRequestDetails (200)
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM requestDetails`).Scan(&count); err != nil {
		t.Fatalf("count requestDetails: %v", err)
	}
	if count != defaultMaxRequestDetails {
		t.Fatalf("expected exactly %d requestDetails after trimming, got %d", defaultMaxRequestDetails, count)
	}

	// Verify that the oldest 20 records (0..19) were trimmed, and newest records (20..219) remain
	var oldestID string
	if err := d.conn.QueryRow(`SELECT id FROM requestDetails ORDER BY timestamp ASC, id ASC LIMIT 1`).Scan(&oldestID); err != nil {
		t.Fatalf("query oldest ID: %v", err)
	}
	if oldestID != "rd-seq-0020" {
		t.Fatalf("expected oldest surviving ID to be rd-seq-0020, got %s", oldestID)
	}

	// 3. Test concurrent writes with 50 goroutines
	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rd := &RequestDetail{
				ID:        fmt.Sprintf("rd-concurrent-%04d", idx),
				Timestamp: baseTime.Add(time.Duration(220+idx) * time.Minute).Format(time.RFC3339),
				Model:     "gpt-4o",
				Status:    "ok",
				Data:      fmt.Sprintf(`{"concurrent":%d}`, idx),
			}
			if err := d.SaveRequestDetail(rd); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent SaveRequestDetail failed: %v", err)
	}

	// Count must be exactly defaultMaxRequestDetails (200)
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM requestDetails`).Scan(&count); err != nil {
		t.Fatalf("count requestDetails: %v", err)
	}
	if count != defaultMaxRequestDetails {
		t.Fatalf("expected exactly %d requestDetails after concurrent writes, got %d", defaultMaxRequestDetails, count)
	}

	// Oldest 70 records (220 + 50 = 270 total, 270 - 200 = 70 trimmed) should be gone, oldest surviving is rd-seq-0070
	if err := d.conn.QueryRow(`SELECT id FROM requestDetails ORDER BY timestamp ASC, id ASC LIMIT 1`).Scan(&oldestID); err != nil {
		t.Fatalf("query oldest ID after concurrent writes: %v", err)
	}
	if oldestID != "rd-seq-0070" {
		t.Fatalf("expected oldest surviving ID after concurrent writes to be rd-seq-0070, got %s", oldestID)
	}
}
