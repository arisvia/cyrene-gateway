package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenance(t *testing.T) {
	d := setupTestDB(t)

	// Determine DB path from connection or temp
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-maint.sqlite")
	maintDB, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open maint db: %v", err)
	}
	defer maintDB.Close()

	// Seed some usageHistory and requestDetails
	now := time.Now().UTC()
	oldTime := now.AddDate(0, 0, -40).Format(time.RFC3339)
	recentTime := now.AddDate(0, 0, -2).Format(time.RFC3339)

	// Insert old and recent usage
	_, err = maintDB.conn.Exec(`INSERT INTO usageHistory (timestamp, provider, model, status, promptTokens, completionTokens, cost) VALUES (?, 'openai', 'gpt-4o', 'success', 10, 20, 0.001);`, oldTime)
	if err != nil {
		t.Fatalf("insert old usage: %v", err)
	}
	_, err = maintDB.conn.Exec(`INSERT INTO usageHistory (timestamp, provider, model, status, promptTokens, completionTokens, cost) VALUES (?, 'openai', 'gpt-4o', 'success', 10, 20, 0.001);`, recentTime)
	if err != nil {
		t.Fatalf("insert recent usage: %v", err)
	}

	// Insert old and recent requestDetails
	_, err = maintDB.conn.Exec(`INSERT INTO requestDetails (id, timestamp, provider, model, data) VALUES ('req-old', ?, 'openai', 'gpt-4o', '{}');`, oldTime)
	if err != nil {
		t.Fatalf("insert old request detail: %v", err)
	}
	_, err = maintDB.conn.Exec(`INSERT INTO requestDetails (id, timestamp, provider, model, data) VALUES ('req-recent', ?, 'openai', 'gpt-4o', '{}');`, recentTime)
	if err != nil {
		t.Fatalf("insert recent request detail: %v", err)
	}

	// Test GetStorageStats
	stats, err := maintDB.GetStorageStats(dbPath)
	if err != nil {
		t.Fatalf("GetStorageStats error: %v", err)
	}
	if stats.TotalRequests != 2 {
		t.Errorf("expected 2 total requests, got %d", stats.TotalRequests)
	}
	if stats.TotalDetails != 2 {
		t.Errorf("expected 2 total details, got %d", stats.TotalDetails)
	}
	if stats.PageSize <= 0 {
		t.Errorf("expected page size > 0, got %d", stats.PageSize)
	}

	// Test CheckpointWAL
	if err := maintDB.CheckpointWAL(); err != nil {
		t.Errorf("CheckpointWAL failed: %v", err)
	}

	// Test Vacuum
	if err := maintDB.Vacuum(); err != nil {
		t.Errorf("Vacuum failed: %v", err)
	}

	// Test PruneLogs with 30 days retention (should prune the 40-day old ones, keep 2-day old ones)
	prunedHist, prunedDet, err := maintDB.PruneLogs(30)
	if err != nil {
		t.Fatalf("PruneLogs failed: %v", err)
	}
	if prunedHist != 1 {
		t.Errorf("expected 1 pruned history, got %d", prunedHist)
	}
	if prunedDet != 1 {
		t.Errorf("expected 1 pruned detail, got %d", prunedDet)
	}

	// Verify counts after prune
	statsAfter, err := maintDB.GetStorageStats(dbPath)
	if err != nil {
		t.Fatalf("GetStorageStats after prune error: %v", err)
	}
	if statsAfter.TotalRequests != 1 {
		t.Errorf("expected 1 request remaining, got %d", statsAfter.TotalRequests)
	}
	if statsAfter.TotalDetails != 1 {
		t.Errorf("expected 1 detail remaining, got %d", statsAfter.TotalDetails)
	}

	// Test invalid retention days
	if _, _, err := maintDB.PruneLogs(0); err == nil {
		t.Errorf("expected error for non-positive retention days")
	}

	_ = d // keep d used
	_ = os.DevNull
}
