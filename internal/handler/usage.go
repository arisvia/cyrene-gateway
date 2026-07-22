package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
)

// handleUsageStats returns aggregated usage statistics.
func (s *Server) handleUsageStats(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}

	totalLifetime, _ := s.DB.GetTotalRequestsLifetime()
	last10, _ := s.DB.GetUsageLast10Minutes()

	stats := map[string]any{
		"totalRequestsLifetime": totalLifetime,
		"last10Minutes":         last10,
		"period":                period,
	}

	// Aggregate from daily data
	var cutoffKey string
	switch period {
	case "7d":
		cutoffKey = time.Now().UTC().AddDate(0, 0, -6).Format("2006-01-02")
	case "30d":
		cutoffKey = time.Now().UTC().AddDate(0, 0, -29).Format("2006-01-02")
	case "60d":
		cutoffKey = time.Now().UTC().AddDate(0, 0, -59).Format("2006-01-02")
	case "24h", "today":
		cutoffKey = time.Now().UTC().Format("2006-01-02")
	default:
		cutoffKey = ""
	}

	days, err := s.DB.GetUsageDailyRange(cutoffKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get usage data"})
		return
	}

	totalRequests := 0
	totalPrompt := 0
	totalCompletion := 0
	totalCost := 0.0
	byProvider := make(map[string]db.DayCounter)
	byModel := make(map[string]db.DayCounter)

	for _, day := range days {
		totalRequests += day.Requests
		totalPrompt += day.PromptTokens
		totalCompletion += day.CompletionTokens
		totalCost += day.Cost

		for k, v := range day.ByProvider {
			p := byProvider[k]
			p.Requests += v.Requests
			p.PromptTokens += v.PromptTokens
			p.CompletionTokens += v.CompletionTokens
			p.Cost += v.Cost
			byProvider[k] = p
		}
		for k, v := range day.ByModel {
			m := byModel[k]
			m.Requests += v.Requests
			m.PromptTokens += v.PromptTokens
			m.CompletionTokens += v.CompletionTokens
			m.Cost += v.Cost
			byModel[k] = m
		}
	}

	stats["totalRequests"] = totalRequests
	stats["totalPromptTokens"] = totalPrompt
	stats["totalCompletionTokens"] = totalCompletion
	stats["totalCost"] = totalCost
	stats["byProvider"] = byProvider
	stats["byModel"] = byModel

	writeJSON(w, http.StatusOK, stats)
}

// handleUsageHistory returns raw usage history records.
func (s *Server) handleUsageHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := db.UsageFilter{
		Provider:  q.Get("provider"),
		Model:     q.Get("model"),
		StartDate: q.Get("startDate"),
		EndDate:   q.Get("endDate"),
		Limit:     limit,
		Offset:    offset,
	}

	entries, err := s.DB.GetUsageHistory(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get usage history"})
		return
	}

	if entries == nil {
		entries = []db.UsageEntry{}
	}

	writeJSON(w, http.StatusOK, entries)
}

// handleUsageChart returns chart data (tokens/cost per day or per hour).
func (s *Server) handleUsageChart(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}

	var bucketCount int
	switch period {
	case "today":
		bucketCount = 24
	case "24h":
		bucketCount = 24
	case "7d":
		bucketCount = 7
	case "30d":
		bucketCount = 30
	case "60d":
		bucketCount = 60
	default:
		bucketCount = 7
	}

	if period == "today" || period == "24h" {
		// Hourly buckets from usage history
		now := time.Now().UTC()
		var startTime time.Time
		if period == "today" {
			startTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		} else {
			startTime = now.Add(-24 * time.Hour)
		}

		entries, err := s.DB.GetUsageHistory(db.UsageFilter{
			StartDate: startTime.Format(time.RFC3339),
			Limit:     10000,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get chart data"})
			return
		}

		buckets := make([]map[string]any, bucketCount)
		for i := range buckets {
			buckets[i] = map[string]any{"label": startTime.Add(time.Duration(i) * time.Hour).Format("15:04"), "tokens": 0, "cost": 0.0}
		}

		for _, e := range entries {
			t, err := time.Parse(time.RFC3339, e.Timestamp)
			if err != nil {
				continue
			}
			idx := int(t.Sub(startTime).Hours())
			if idx >= 0 && idx < bucketCount {
				buckets[idx]["tokens"] = buckets[idx]["tokens"].(int) + e.PromptTokens + e.CompletionTokens
				buckets[idx]["cost"] = buckets[idx]["cost"].(float64) + e.Cost
			}
		}

		writeJSON(w, http.StatusOK, buckets)
		return
	}

	// Daily buckets from usageDaily
	now := time.Now().UTC()
	days, err := s.DB.GetUsageDailyRange(now.AddDate(0, 0, -(bucketCount - 1)).Format("2006-01-02"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get chart data"})
		return
	}

	buckets := make([]map[string]any, bucketCount)
	for i := 0; i < bucketCount; i++ {
		d := now.AddDate(0, 0, -(bucketCount - 1 - i))
		dateKey := d.Format("2006-01-02")
		label := d.Format("Jan 02")
		tokens := 0
		cost := 0.0
		if day, ok := days[dateKey]; ok {
			tokens = day.PromptTokens + day.CompletionTokens
			cost = day.Cost
		}
		buckets[i] = map[string]any{"label": label, "tokens": tokens, "cost": cost}
	}

	writeJSON(w, http.StatusOK, buckets)
}

// handleUsageRequestDetails returns paginated request details.
func (s *Server) handleUsageRequestDetails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))

	filter := db.RequestDetailFilter{
		Provider:     q.Get("provider"),
		Model:        q.Get("model"),
		ConnectionID: q.Get("connectionId"),
		Status:       q.Get("status"),
		StartDate:    q.Get("startDate"),
		EndDate:      q.Get("endDate"),
		Page:         page,
		PageSize:     pageSize,
	}

	result, err := s.DB.GetRequestDetails(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get request details"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleUsageRequestDetailByID returns a single request detail.
func (s *Server) handleUsageRequestDetailByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	detail, err := s.DB.GetRequestDetailByID(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "request detail not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(detail)
}
