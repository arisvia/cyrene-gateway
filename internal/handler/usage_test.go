package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// Usage rows are written with the canonical provider id and the filter sits on
// the admin surface, so it matches that id exactly: a shorthand is a client-side
// model-prefix convenience and must not become a second key for stats.
func TestUsageHistoryFilterIsCanonicalIDOnly(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	srv := &Server{DB: database}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := database.SaveUsageEntry(&db.UsageEntry{
		Timestamp: now, Provider: "claude", Model: "sonnet", APIKey: "k", Endpoint: "/v1/messages",
	}); err != nil {
		t.Fatal(err)
	}

	history := func(query string) string {
		t.Helper()
		w := httptest.NewRecorder()
		srv.handleUsageHistory(w, httptest.NewRequest("GET", "/api/usage/history?"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	if got := history("provider=claude"); !strings.Contains(got, "\"sonnet\"") {
		t.Fatalf("canonical filter lost its own row: %s", got)
	}
	if got := history("provider=cc"); strings.Contains(got, "\"sonnet\"") {
		t.Errorf("alias filter matched canonical rows, want the id taken verbatim: %s", got)
	}
}

func TestUsageStatsCacheCollapsesRepeatedReads(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		database, err := db.Open(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		srv := &Server{DB: database}

		save := func(model string) {
			t.Helper()
			if err := database.SaveUsageEntry(&db.UsageEntry{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Provider:  "p", Model: model, APIKey: "k", Endpoint: "/v1/messages",
				PromptTokens: 1, CompletionTokens: 1,
			}); err != nil {
				t.Fatal(err)
			}
		}
		fetch := func() (string, string) {
			t.Helper()
			w := httptest.NewRecorder()
			srv.handleUsageStats(w, httptest.NewRequest("GET", "/api/usage/stats?period=7d", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			return w.Body.String(), w.Header().Get("X-Cyrene-Cache")
		}

		save("first")
		body, cache := fetch()
		if cache != "" {
			t.Errorf("first read served from cache: %q", cache)
		}
		if !strings.Contains(body, "first") {
			t.Fatalf("first read lost its data: %s", body)
		}

		// A second read inside the TTL must reuse the payload even though the
		// underlying rows changed, otherwise the cache buys nothing.
		save("second")
		body, cache = fetch()
		if cache != "hit" {
			t.Errorf("second read missed cache, header %q", cache)
		}
		if strings.Contains(body, "second") {
			t.Errorf("cached payload recomputed: %s", body)
		}

		// Once the TTL lapses the new row must appear.
		time.Sleep(usageStatsTTL + time.Second)
		body, cache = fetch()
		if cache != "" {
			t.Errorf("read after TTL still cached: %q", cache)
		}
		if !strings.Contains(body, "second") {
			t.Errorf("stale payload survived the TTL: %s", body)
		}
	})
}
