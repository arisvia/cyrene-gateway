package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
)

func TestUsageStatsPeriodWindows(t *testing.T) {
	for _, period := range []string{"today", "24h", "7d", "30d", "60d"} {
		t.Run(period, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				database, err := db.Open(":memory:")
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
				srv := &Server{DB: database}
				now := time.Now().UTC()
				start := now.Truncate(24 * time.Hour)
				switch period {
				case "24h":
					start = now.Add(-24 * time.Hour)
				case "7d":
					start = start.AddDate(0, 0, -6)
				case "30d":
					start = start.AddDate(0, 0, -29)
				case "60d":
					start = start.AddDate(0, 0, -59)
				}
				for _, ts := range []time.Time{start.Add(-time.Second), start, now} {
					if err := database.SaveUsageEntry(&db.UsageEntry{
						Timestamp: ts.Format(time.RFC3339Nano), Provider: "p", Model: "m", APIKey: "k", Endpoint: "/v1/messages",
						PromptTokens: 10, CompletionTokens: 5, Cost: 0.25,
					}); err != nil {
						t.Fatal(err)
					}
				}
				w := httptest.NewRecorder()
				srv.handleUsageStats(w, httptest.NewRequest("GET", "/api/usage/stats?period="+period, nil))
				if w.Code != http.StatusOK {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				var result struct {
					TotalRequests         int                      `json:"totalRequests"`
					TotalPromptTokens     int                      `json:"totalPromptTokens"`
					TotalCompletionTokens int                      `json:"totalCompletionTokens"`
					TotalCost             float64                  `json:"totalCost"`
					ByProvider            map[string]db.DayCounter `json:"byProvider"`
					ByModel               map[string]db.DayCounter `json:"byModel"`
					ByKey                 map[string]db.DayCounter `json:"byKey"`
					ByEndpoint            map[string]db.DayCounter `json:"byEndpoint"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.TotalRequests != 2 || result.TotalPromptTokens != 20 || result.TotalCompletionTokens != 10 || result.TotalCost != 0.5 {
					t.Errorf("incorrect %s totals: %+v", period, result)
				}
				for _, counter := range []db.DayCounter{result.ByProvider["p"], result.ByModel["m|p"], result.ByKey["k"], result.ByEndpoint["/v1/messages"]} {
					if counter.Requests != 2 || counter.PromptTokens != 20 || counter.CompletionTokens != 10 || counter.Cost != 0.5 {
						t.Errorf("%s dimension cutoff differs: %+v", period, counter)
					}
				}
			})
		})
	}
}

func TestUsageStatsPrunedHistoryIsExplicit(t *testing.T) {
	for _, period := range []string{"today", "24h", "7d", "all"} {
		t.Run(period, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				database, err := db.Open(":memory:")
				if err != nil {
					t.Fatal(err)
				}
				defer database.Close()
				now := time.Now().UTC()
				for _, key := range []string{"kept", "pruned"} {
					if err := database.SaveUsageEntry(&db.UsageEntry{Timestamp: now.Format(time.RFC3339), Provider: "p", Model: "m", APIKey: key}); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := database.Conn().Exec(`DELETE FROM usageHistory WHERE apiKey = 'pruned'`); err != nil {
					t.Fatal(err)
				}
				w := httptest.NewRecorder()
				(&Server{DB: database}).handleUsageStats(w, httptest.NewRequest("GET", "/api/usage/stats?period="+period, nil))
				var result struct {
					TotalRequests       int                      `json:"totalRequests"`
					HistoryIncomplete   bool                     `json:"historyIncomplete"`
					HistoryMissingDates []string                 `json:"historyMissingDates"`
					TotalsSource        string                   `json:"totalsSource"`
					ByKey               map[string]db.DayCounter `json:"byKey"`
				}
				if w.Code != http.StatusOK {
					t.Fatalf("status %d: %s", w.Code, w.Body.String())
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				want, source := 2, "daily"
				if period == "24h" {
					want, source = 1, "history"
				}
				if result.TotalRequests != want || result.TotalsSource != source || !result.HistoryIncomplete || len(result.HistoryMissingDates) != 1 || result.HistoryMissingDates[0] != "2000-01-01" {
					t.Errorf("pruned coverage not represented: %+v", result)
				}
				if len(result.ByKey) != 1 || result.ByKey["kept"].Requests != 1 {
					t.Errorf("invented dimension counts: %+v", result.ByKey)
				}
			})
		})
	}
}

func TestUsageStatsRollingWindowFractionalAndOffsetTimestamps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		database, err := db.Open(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		// Fake clock starts at 2000-01-01T00:00:00Z. Include the exact
		// lower boundary and its offset equivalent, not the past/future.
		for _, ts := range []string{"1999-12-31T00:00:00Z", "1999-12-31T01:00:00+01:00", "1999-12-30T23:59:59.999999999Z", "2000-01-01T00:00:00.000000001Z"} {
			if err := database.SaveUsageEntry(&db.UsageEntry{Timestamp: ts, Provider: "p", Model: "m"}); err != nil {
				t.Fatal(err)
			}
		}
		w := httptest.NewRecorder()
		(&Server{DB: database}).handleUsageStats(w, httptest.NewRequest("GET", "/api/usage/stats?period=24h", nil))
		var result struct {
			TotalRequests int                      `json:"totalRequests"`
			ByKey         map[string]db.DayCounter `json:"byKey"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || result.TotalRequests != 2 || result.ByKey["default"].Requests != 2 {
			t.Fatalf("rolling range not precise: %s", w.Body.String())
		}
	})
}
