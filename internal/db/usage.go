package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UsageEntry represents a row in usageHistory.
type UsageEntry struct {
	Endpoint         string  `json:"endpoint,omitempty"`
	Timestamp        string  `json:"timestamp"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	ConnectionID     string  `json:"connectionId,omitempty"`
	APIKey           string  `json:"apiKey,omitempty"`
	Status           string  `json:"status"`
	Tokens           string  `json:"tokens,omitempty"`
	Meta             string  `json:"meta,omitempty"`
	ID               int64   `json:"id,omitempty"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Cost             float64 `json:"cost"`
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
	ByProvider       map[string]DayCounter `json:"byProvider"`
	ByModel          map[string]DayCounter `json:"byModel"`
	Requests         int                   `json:"requests"`
	PromptTokens     int                   `json:"promptTokens"`
	CompletionTokens int                   `json:"completionTokens"`
	Cost             float64               `json:"cost"`
}

// SaveUsageEntry records a new event on every call, regardless of e.ID or equal content.
// History, daily aggregation and the lifetime counter are committed atomically.
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
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query daily: %w", err)
	}
	if err == nil {
		if jsonErr := json.Unmarshal([]byte(dayDataStr), &day); jsonErr != nil {
			day = DayData{}
		}
	}

	addUsageToDay(&day, *e)

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

func addUsageToDay(day *DayData, e UsageEntry) {
	if day.ByProvider == nil {
		day.ByProvider = make(map[string]DayCounter)
	}
	if day.ByModel == nil {
		day.ByModel = make(map[string]DayCounter)
	}
	day.Requests++
	day.PromptTokens += e.PromptTokens
	day.CompletionTokens += e.CompletionTokens
	day.Cost += e.Cost
	if e.Provider != "" {
		day.ByProvider[e.Provider] = addUsageToCounter(day.ByProvider[e.Provider], e)
	}
	modelKey := e.Model
	if e.Provider != "" {
		modelKey += "|" + e.Provider
	}
	day.ByModel[modelKey] = addUsageToCounter(day.ByModel[modelKey], e)
}

func addUsageToCounter(c DayCounter, e UsageEntry) DayCounter {
	c.Requests++
	c.PromptTokens += e.PromptTokens
	c.CompletionTokens += e.CompletionTokens
	c.Cost += e.Cost
	return c
}

const usageHistoryColumns = `id, timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta`

// GetUsageHistory returns usage records with optional filters.
func (d *DB) GetUsageHistory(f UsageFilter) ([]UsageEntry, error) {
	var query strings.Builder
	query.WriteString(`SELECT ` + usageHistoryColumns + ` FROM usageHistory`)
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
				query.WriteString(" WHERE " + c)
			} else {
				query.WriteString(" AND " + c)
			}
		}
	}

	query.WriteString(" ORDER BY id DESC")

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	query.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	if f.Offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", f.Offset))
	}

	rows, err := d.conn.Query(query.String(), args...)
	if err != nil {
		return nil, err
	}
	return scanUsageHistory(rows)
}

func scanUsageHistory(rows *sql.Rows) ([]UsageEntry, error) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	for i := range 10 {
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
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var n int64
	fmt.Sscanf(val, "%d", &n)
	return n, nil
}

// GetConnectionUsageCount returns the number of requests for a connection within a period.
// Period can be "daily", "weekly", or "monthly".
func (d *DB) GetConnectionUsageCount(connectionID string, period string) int {
	now := time.Now().UTC()
	var cutoff time.Time
	switch period {
	case "weekly":
		cutoff = now.AddDate(0, 0, -7)
	case "monthly":
		cutoff = now.AddDate(0, -1, 0)
	default: // daily
		cutoff = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	var count int
	err := d.conn.QueryRow(
		`SELECT COUNT(*) FROM usageHistory WHERE connectionId = ? AND timestamp >= ?`,
		connectionID, cutoff.Format(time.RFC3339),
	).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

// GetConnectionUsageTokens returns total tokens used by a connection within a period.
func (d *DB) GetConnectionUsageTokens(connectionID string, period string) (prompt, completion int) {
	now := time.Now().UTC()
	var cutoff time.Time
	switch period {
	case "weekly":
		cutoff = now.AddDate(0, 0, -7)
	case "monthly":
		cutoff = now.AddDate(0, -1, 0)
	default: // daily
		cutoff = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	err := d.conn.QueryRow(
		`SELECT COALESCE(SUM(promptTokens),0), COALESCE(SUM(completionTokens),0) FROM usageHistory WHERE connectionId = ? AND timestamp >= ?`,
		connectionID, cutoff.Format(time.RFC3339),
	).Scan(&prompt, &completion)
	if err != nil {
		return 0, 0
	}
	return prompt, completion
}

// UsageSummary aggregates retained history without a pagination limit. Day counts
// cover the entire first UTC day so callers can detect missing/pruned history.
// They do not imply that the missing events fall inside a partial-day window.
type UsageSummary struct {
	DayData
	ByKey         map[string]DayCounter
	ByEndpoint    map[string]DayCounter
	RequestsByDay map[string]int
}

func (d *DB) GetUsageSummary(start, end time.Time) (*UsageSummary, error) {
	query := `SELECT timestamp, COALESCE(provider,''), COALESCE(model,''),
		COALESCE(apiKey,''), COALESCE(endpoint,''), promptTokens, completionTokens, cost FROM usageHistory WHERE 1=1`
	var args []any
	var dayStart time.Time
	if !start.IsZero() {
		dayStart = start.UTC().Truncate(24 * time.Hour)
		// SQLite date comparisons handle offsets. Widen by a second, then filter
		// in Go so SQLite's millisecond rounding never decides exact boundaries.
		query += ` AND julianday(timestamp) >= julianday(?)`
		args = append(args, dayStart.Add(-time.Second).Format(time.RFC3339))
	}
	if !end.IsZero() {
		query += ` AND julianday(timestamp) <= julianday(?)`
		args = append(args, end.UTC().Add(time.Second).Format(time.RFC3339Nano))
	}
	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &UsageSummary{
		DayData: DayData{ByProvider: make(map[string]DayCounter), ByModel: make(map[string]DayCounter)},
		ByKey:   make(map[string]DayCounter), ByEndpoint: make(map[string]DayCounter), RequestsByDay: make(map[string]int),
	}
	for rows.Next() {
		var e UsageEntry
		if err := rows.Scan(&e.Timestamp, &e.Provider, &e.Model, &e.APIKey, &e.Endpoint, &e.PromptTokens, &e.CompletionTokens, &e.Cost); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("invalid usage timestamp %q: %w", e.Timestamp, err)
		}
		if (!dayStart.IsZero() && ts.Before(dayStart)) || (!end.IsZero() && ts.After(end)) {
			continue
		}
		result.RequestsByDay[ts.UTC().Format("2006-01-02")]++
		if !start.IsZero() && ts.Before(start) {
			continue
		}
		addUsageToDay(&result.DayData, e)
		key, endpoint := e.APIKey, e.Endpoint
		if key == "" {
			key = "default"
		}
		if endpoint == "" {
			endpoint = "other"
		}
		result.ByKey[key] = addUsageToCounter(result.ByKey[key], e)
		result.ByEndpoint[endpoint] = addUsageToCounter(result.ByEndpoint[endpoint], e)
	}
	return result, rows.Err()
}

// GetUsageDimensions returns aggregations by apiKey and by endpoint for records on or after cutoffTime.
func (d *DB) GetUsageDimensions(cutoffTime string) (byKey map[string]DayCounter, byEndpoint map[string]DayCounter, err error) {
	var start time.Time
	if cutoffTime != "" {
		start, err = time.Parse(time.RFC3339, cutoffTime)
		if err != nil {
			return nil, nil, err
		}
	}
	summary, err := d.GetUsageSummary(start, time.Time{})
	if err != nil {
		return nil, nil, err
	}
	return summary.ByKey, summary.ByEndpoint, nil
}

func dateKeyFromTimestamp(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Now().UTC().Format("2006-01-02")
	}
	return t.UTC().Format("2006-01-02")
}
