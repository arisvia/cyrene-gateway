package db

import (
	"fmt"
	"os"
	"time"
)

// StorageStats holds filesystem and SQLite storage metrics.
type StorageStats struct {
	DBSizeBytes   int64 `json:"dbSizeBytes"`
	WALSizeBytes  int64 `json:"walSizeBytes"`
	PageCount     int64 `json:"pageCount"`
	PageSize      int64 `json:"pageSize"`
	TotalRequests int64 `json:"totalRequests"`
	TotalDetails  int64 `json:"totalDetails"`
}

// GetStorageStats queries disk file sizes and SQLite table stats.
func (d *DB) GetStorageStats(dbPath string) (*StorageStats, error) {
	stats := &StorageStats{}

	if fi, err := os.Stat(dbPath); err == nil {
		stats.DBSizeBytes = fi.Size()
	}

	walPath := dbPath + "-wal"
	if fi, err := os.Stat(walPath); err == nil {
		stats.WALSizeBytes = fi.Size()
	}

	// Query PRAGMA metrics
	_ = d.conn.QueryRow("PRAGMA page_count;").Scan(&stats.PageCount)
	_ = d.conn.QueryRow("PRAGMA page_size;").Scan(&stats.PageSize)

	// Query row counts
	_ = d.conn.QueryRow("SELECT COUNT(*) FROM usageHistory;").Scan(&stats.TotalRequests)
	_ = d.conn.QueryRow("SELECT COUNT(*) FROM requestDetails;").Scan(&stats.TotalDetails)

	return stats, nil
}

// CheckpointWAL forces SQLite to flush WAL pages into the database file and truncate WAL.
func (d *DB) CheckpointWAL() error {
	_, err := d.conn.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	if err != nil {
		return fmt.Errorf("wal checkpoint truncate: %w", err)
	}
	return nil
}

// Vacuum defragments and reclaims unused disk pages from the SQLite database.
func (d *DB) Vacuum() error {
	_, err := d.conn.Exec("VACUUM;")
	if err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}
	return nil
}

// ResetUsage deletes every usage record and the aggregates derived from them.
//
// Destructive and irreversible: this drops the history the stats panels read,
// which is the point -- there is no way to distinguish "reset the counters"
// from "forget everything" once the rows are gone. Callers must confirm first.
// usageDaily and the lifetime counter are included because leaving them would
// keep reporting totals for records that no longer exist.
func (d *DB) ResetUsage() (history, details int64, err error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	resHist, err := tx.Exec("DELETE FROM usageHistory")
	if err != nil {
		return 0, 0, fmt.Errorf("reset usageHistory: %w", err)
	}
	history, _ = resHist.RowsAffected()

	resDetails, err := tx.Exec("DELETE FROM requestDetails")
	if err != nil {
		return history, 0, fmt.Errorf("reset requestDetails: %w", err)
	}
	details, _ = resDetails.RowsAffected()

	if _, err := tx.Exec("DELETE FROM usageDaily"); err != nil {
		return history, details, fmt.Errorf("reset usageDaily: %w", err)
	}
	// The lifetime counter lives in _meta; zero rather than delete so the stat
	// endpoint keeps reading a value instead of falling back on a missing row.
	if _, err := tx.Exec(
		"INSERT INTO _meta (key, value) VALUES ('totalRequestsLifetime', '0') " +
			"ON CONFLICT(key) DO UPDATE SET value = '0'",
	); err != nil {
		return history, details, fmt.Errorf("reset lifetime counter: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit reset: %w", err)
	}
	return history, details, nil
}

// PruneLogs deletes usage history and request details older than retentionDays.
func (d *DB) PruneLogs(retentionDays int) (int64, int64, error) {
	if retentionDays <= 0 {
		return 0, 0, fmt.Errorf("retention days must be positive")
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format(time.RFC3339)

	resHist, err := d.conn.Exec("DELETE FROM usageHistory WHERE timestamp < ?;", cutoff)
	if err != nil {
		return 0, 0, fmt.Errorf("prune usageHistory: %w", err)
	}
	prunedHistory, _ := resHist.RowsAffected()

	resDetails, err := d.conn.Exec("DELETE FROM requestDetails WHERE timestamp < ?;", cutoff)
	if err != nil {
		return prunedHistory, 0, fmt.Errorf("prune requestDetails: %w", err)
	}
	prunedDetails, _ := resDetails.RowsAffected()

	return prunedHistory, prunedDetails, nil
}
