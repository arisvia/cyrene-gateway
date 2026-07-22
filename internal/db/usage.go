package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// UsageEntry represents a row in usageHistory.
type UsageEntry struct {
	ID               int64   `json:"id,omitempty"`
	Timestamp        string  `json:"timestamp"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	ConnectionID     string  `json:"connectionId,omitempty"`
	APIKey           string  `json:"apiKey,omitempty"`
	Endpoint         string  `json:"endpoint,omitempty"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Cost             float64 `json:"cost"`
	Status           string  `json:"status"`
	Tokens           string  `json:"tokens,omitempty"`
	Meta             string  `json:"meta,omitempty"`
}

// UsageFilter for querying usage history.
type UsageFilter struct {
	Provider  string
	Model     string
	StartDate string
	EndDate   string
	Limit     int
	Offset    int
}

// DayCounter is a per-dimension aggregation counter.
type DayCounter struct {
	Requests         int     `json:"requests"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Cost             float64 `json:"cost"`
}

// DayData is the JSON blob stored in usageDaily.
type DayData struct {
	Requests         int                   `json:"requests"`
	PromptTokens     int                   `json:"promptTokens"`
	CompletionTokens int                   `json:"completionTokens"`
	Cost             float64               `json:"cost"`
	ByProvider       map[string]DayCounter `json:"byProvider"`
	ByModel          map[string]DayCounter `json:"byModel"`
}

// SaveUsageEntry inserts a usage record and updates the daily aggregation in one transaction.
func (d *DB) SaveUsageEntry(e *UsageEntry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if e.Status == "" {
		e.Status = "ok"
	}
	if e.Tokens == "" {
		tokens := map[string]int{
			"prompt_tokens":     e.PromptTokens,
			"completion_tokens": e.CompletionTokens,
			"total_tokens":      e.PromptTokens + e.CompletionTokens,
		}
		b, _ := json.Marshal(tokens)
		e.Tokens = string(b)
	}
	if e.Meta == "" {
		e.Meta = "{}"
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Dedup check: same timestamp+provider+model+connection+tokens
	var existingID int64
	err = tx.QueryRow(
		`SELECT id FROM usageHistory
		 WHERE timestamp = ? AND COALESCE(provider,'') = COALESCE(?,'')
		   AND COALESCE(model,'') = COALESCE(?,'')
		   AND COALESCE(connectionId,'') = COALESCE(?,'')
		   AND promptTokens = ? AND completionTokens = ?
		 ORDER BY id DESC LIMIT 1`,
		e.Timestamp, e.Provider, e.Model, e.ConnectionID,
		e.PromptTokens, e.CompletionTokens,
	).Scan(&existingID)
	if err == nil {
		// Duplicate, skip
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("dedup check: %w", err)
	}

	// Insert usage history
	_, err = tx.Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp, e.Provider, e.Model, e.ConnectionID, e.APIKey, e.Endpoint,
		e.PromptTokens, e.CompletionTokens, e.Cost, e.Status, e.Tokens, e.Meta,
	)
	if err != nil {
		return fmt.Errorf("insert usage: %w", err)
	}

	// Update daily aggregation
	dateKey := dateKeyFromTimestamp(e.Timestamp)
	var dayDataStr string
	err = tx.QueryRow(`SELECT data FROM usageDaily WHERE dateKey = ?`, dateKey).Scan(&dayDataStr)

	var day DayData
	if err == sql.ErrNoRows {
		day = DayData{
			ByProvider: make(map[string]DayCounter),
			ByModel:    make(map[string]DayCounter),
		}
	} else if err != nil {
		return fmt.Errorf("query daily: %w", err)
	} else {
		if jsonErr := json.Unmarshal([]byte(dayDataStr), &day); jsonErr != nil {
			day = DayData{
				ByProvider: make(map[string]DayCounter),
				ByModel:    make(map[string]DayCounter),
			}
		}
		if day.ByProvider == nil {
			day.ByProvider = make(map[string]DayCounter)
		}
		if day.ByModel == nil {
			day.ByModel = make(map[string]DayCounter)
		}
	}

	// Aggregate
	day.Requests++
	day.PromptTokens += e.PromptTokens
	day.CompletionTokens += e.CompletionTokens
	day.Cost += e.Cost

	if e.Provider != "" {
		p := day.ByProvider[e.Provider]
		p.Requests++
		p.PromptTokens += e.PromptTokens
		p.CompletionTokens += e.CompletionTokens
		p.Cost += e.Cost
		day.ByProvider[e.Provider] = p
	}

	modelKey := e.Model
	if e.Provider != "" {
		modelKey = e.Model + "|" + e.Provider
	}
	m := day.ByModel[modelKey]
	m.Requests++
	m.PromptTokens += e.PromptTokens
	m.CompletionTokens += e.CompletionTokens
	m.Cost += e.Cost
	day.ByModel[modelKey] = m

	dayBytes, _ := json.Marshal(day)
	_, err = tx.Exec(
		`INSERT INTO usageDaily (dateKey, data) VALUES (?, ?)
		 ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data`,
		dateKey, string(dayBytes),
	)
	if err != nil {
		return fmt.Errorf("upsert daily: %w", err)
	}

	// Increment lifetime counter
	_, err = tx.Exec(
		`INSERT INTO _meta (key, value) VALUES ('totalRequestsLifetime', '1')
		 ON CONFLICT(key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT)`,
	)
	if err != nil {
		return fmt.Errorf("increment lifetime: %w", err)
	}

	return tx.Commit()
}

// GetUsageHistory returns usage records with optional filters.
func (d *DB) GetUsageHistory(f UsageFilter) ([]UsageEntry, error) {
	query := `SELECT id, timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta FROM usageHistory`
	var conds []string
	var args []any

	if f.Provider != "" {
		conds = append(conds, "provider = ?")
		args = append(args, f.Provider)
	}
	if f.Model != "" {
		conds = append(conds, "model = ?")
		args = append(args, f.Model)
	}
	if f.StartDate != "" {
		conds = append(conds, "timestamp >= ?")
		args = append(args, f.StartDate)
	}
	if f.EndDate != "" {
		conds = append(conds, "timestamp <= ?")
		args = append(args, f.EndDate)
	}

	if len(conds) > 0 {
		for i, c := range conds {
			if i == 0 {
				query += " WHERE " + c
			} else {
				query += " AND " + c
			}
		}
	}

	query += " ORDER BY id DESC"

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)
	if f.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", f.Offset)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []UsageEntry
	for rows.Next() {
		var e UsageEntry
		var provider, model, connID, apiKey, endpoint, tokens, meta sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &provider, &model, &connID, &apiKey, &endpoint,
			&e.PromptTokens, &e.CompletionTokens, &e.Cost, &e.Status, &tokens, &meta); err != nil {
			return nil, err
		}
		e.Provider = provider.String
		e.Model = model.String
		e.ConnectionID = connID.String
		e.APIKey = apiKey.String
		e.Endpoint = endpoint.String
		e.Tokens = tokens.String
		e.Meta = meta.String
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetUsageDaily returns the aggregated day data for a date key (YYYY-MM-DD).
func (d *DB) GetUsageDaily(dateKey string) (*DayData, error) {
	var data string
	err := d.conn.QueryRow(`SELECT data FROM usageDaily WHERE dateKey = ?`, dateKey).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var day DayData
	if err := json.Unmarshal([]byte(data), &day); err != nil {
		return nil, err
	}
	return &day, nil
}

// GetUsageDailyRange returns all daily records from cutoffKey onwards.
func (d *DB) GetUsageDailyRange(cutoffKey string) (map[string]DayData, error) {
	query := `SELECT dateKey, data FROM usageDaily`
	var args []any
	if cutoffKey != "" {
		query += ` WHERE dateKey >= ?`
		args = append(args, cutoffKey)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]DayData)
	for rows.Next() {
		var dateKey, data string
		if err := rows.Scan(&dateKey, &data); err != nil {
			return nil, err
		}
		var day DayData
		if err := json.Unmarshal([]byte(data), &day); err != nil {
			continue
		}
		result[dateKey] = day
	}
	return result, rows.Err()
}

// GetUsageLast10Minutes returns per-minute buckets for the last 10 minutes.
func (d *DB) GetUsageLast10Minutes() ([]map[string]any, error) {
	now := time.Now().UTC()
	currentMinute := now.Truncate(time.Minute)
	tenMinAgo := currentMinute.Add(-9 * time.Minute)

	rows, err := d.conn.Query(
		`SELECT timestamp, promptTokens, completionTokens, cost FROM usageHistory WHERE timestamp >= ? AND timestamp <= ?`,
		tenMinAgo.Format(time.RFC3339), now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Build 10 buckets
	buckets := make([]map[string]any, 10)
	bucketMap := make(map[int64]int) // minute epoch → bucket index
	for i := 0; i < 10; i++ {
		ts := currentMinute.Add(time.Duration(i-9) * time.Minute)
		epoch := ts.Unix() / 60
		bucketMap[epoch] = i
		buckets[i] = map[string]any{
			"requests":         0,
			"promptTokens":     0,
			"completionTokens": 0,
			"cost":             0.0,
		}
	}

	for rows.Next() {
		var ts string
		var prompt, completion int
		var cost float64
		if err := rows.Scan(&ts, &prompt, &completion, &cost); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		minuteEpoch := t.Unix() / 60
		if idx, ok := bucketMap[minuteEpoch]; ok {
			buckets[idx]["requests"] = buckets[idx]["requests"].(int) + 1
			buckets[idx]["promptTokens"] = buckets[idx]["promptTokens"].(int) + prompt
			buckets[idx]["completionTokens"] = buckets[idx]["completionTokens"].(int) + completion
			buckets[idx]["cost"] = buckets[idx]["cost"].(float64) + cost
		}
	}
	return buckets, nil
}

// GetTotalRequestsLifetime returns the lifetime request counter.
func (d *DB) GetTotalRequestsLifetime() (int64, error) {
	var val string
	err := d.conn.QueryRow(`SELECT value FROM _meta WHERE key = 'totalRequestsLifetime'`).Scan(&val)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var n int64
	fmt.Sscanf(val, "%d", &n)
	return n, nil
}

func dateKeyFromTimestamp(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Now().UTC().Format("2006-01-02")
	}
	return t.Format("2006-01-02")
}
