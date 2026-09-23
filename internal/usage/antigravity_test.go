package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClusterAntigravityQuotas(t *testing.T) {
	rem95 := 0.95
	rem100 := 1.0

	resp := AntigravityModelsResponse{
		Models: map[string]struct {
			DisplayName string `json:"displayName"`
			QuotaInfo   *struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
				IsExhausted       bool     `json:"isExhausted"`
			} `json:"quotaInfo"`
		}{
			// Gemini text family models
			"gemini-3.8-flash-high": {
				DisplayName: "Gemini 3.8 Flash (High)",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem95,
					ResetTime:         "2026-09-10T14:21:00Z",
				},
			},
			"gemini-3.8-flash-medium": {
				DisplayName: "Gemini 3.8 Flash (Medium)",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem95,
					ResetTime:         "2026-09-10T14:21:00Z",
				},
			},
			"gemini-3.7-flash-high": {
				DisplayName: "Gemini 3.7 Flash (High)",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem95,
					ResetTime:         "2026-09-10T14:21:00Z",
				},
			},
			// Claude family models
			"claude-sonnet-4-6": {
				DisplayName: "Claude Sonnet 4.6 (Thinking)",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem100,
					ResetTime:         "2026-09-10T15:00:00Z",
				},
			},
			"claude-opus-4-6-thinking": {
				DisplayName: "Claude Opus 4.6 (Thinking)",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem100,
					ResetTime:         "2026-09-10T15:00:00Z",
				},
			},
			// Standalone model
			"gpt-oss-120b-medium": {
				DisplayName: "GPT-OSS 120B (Medium)",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem100,
					ResetTime:         "2026-09-10T15:00:00Z",
				},
			},
			// Dedicated image model
			"imagen-3.0-generate": {
				DisplayName: "Imagen 3.0",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem95,
					ResetTime:         "2026-09-10T14:21:00Z",
				},
			},
			// Internal / completion model to ignore
			"chat_experimental_internal": {
				DisplayName: "Internal Test",
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem95,
				},
			},
		},
	}

	quotas := clusterAntigravityQuotas(resp)

	// Claude and GPT-OSS are now clustered together in the shared pool, plus Gemini and Imagen = 3 buckets
	if len(quotas) != 3 {
		t.Fatalf("expected exactly 3 clustered quota buckets, got %d: %+v", len(quotas), quotas)
	}

	geminiQ, ok := quotas["gemini"]
	if !ok {
		t.Fatalf("missing 'gemini' quota")
	}
	if geminiQ.DisplayName != "Gemini (Flash / Pro / Image / Search)" {
		t.Errorf("expected 'Gemini (Flash / Pro / Image / Search)', got %q", geminiQ.DisplayName)
	}
	if geminiQ.Used != 50 || geminiQ.Remaining != 950 || geminiQ.RemainingPercentage != 95.0 {
		t.Errorf("unexpected gemini quota math: %+v", geminiQ)
	}

	claudeQ, ok := quotas["claude"]
	if !ok {
		t.Fatalf("missing 'claude' quota")
	}
	if claudeQ.DisplayName != "Claude & GPT (Shared)" {
		t.Errorf("expected 'Claude & GPT (Shared)', got %q", claudeQ.DisplayName)
	}
	if claudeQ.Used != 0 || claudeQ.Remaining != 1000 || claudeQ.RemainingPercentage != 100.0 {
		t.Errorf("unexpected claude quota math: %+v", claudeQ)
	}
	imgQ, ok := quotas["imagen-3.0-generate"]
	if !ok {
		t.Fatalf("missing 'imagen-3.0-generate' quota")
	}
	if imgQ.DisplayName != "Imagen 3.0" {
		t.Errorf("expected 'Imagen 3.0', got %q", imgQ.DisplayName)
	}
}
func TestClusterAntigravityQuotas_ResetTimeNormalization(t *testing.T) {
	rem := 0.5
	resp := AntigravityModelsResponse{
		Models: map[string]struct {
			DisplayName string `json:"displayName"`
			QuotaInfo   *struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
				IsExhausted       bool     `json:"isExhausted"`
			} `json:"quotaInfo"`
		}{
			"gemini-2.5-flash": {
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem,
					ResetTime:         "即将重置",
				},
			},
			"claude-3-7-sonnet": {
				QuotaInfo: &struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					IsExhausted       bool     `json:"isExhausted"`
				}{
					RemainingFraction: &rem,
					ResetTime:         "not-a-valid-time",
				},
			},
		},
	}

	quotas := clusterAntigravityQuotas(resp)
	geminiQ := quotas["gemini"]
	if !geminiQ.ResetSoon {
		t.Errorf("expected ResetSoon=true for 即将重置 sentinel, got false")
	}
	if geminiQ.ResetAt != "" {
		t.Errorf("expected empty ResetAt on sentinel text, got %q", geminiQ.ResetAt)
	}

	claudeQ := quotas["claude"]
	if claudeQ.ResetAt != "" {
		t.Errorf("expected empty ResetAt for unparseable time string, got %q", claudeQ.ResetAt)
	}
}

func TestFetchAntigravity_MockServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1internal:fetchAvailableModels" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"models": {
				"gemini-3.8-flash-high": {
					"displayName": "Gemini 3.8 Flash",
					"quotaInfo": {
						"remainingFraction": 0.8,
						"resetTime": "2026-09-10T18:00:00Z"
					}
				},
				"claude-sonnet-4-6": {
					"displayName": "Claude Sonnet 4.6",
					"quotaInfo": {
						"remainingFraction": 1.0,
						"resetTime": "2026-09-10T19:00:00Z"
					}
				}
			}
		}`))
	}))
	defer ts.Close()

	res := FetchQuota(context.Background(), ts.Client(), QuotaCredentials{
		Provider:    "antigravity",
		AccessToken: "mock-token",
		BaseURL:     ts.URL,
	})

	if len(res.Quotas) != 2 {
		t.Fatalf("expected 2 quotas, got %d: %+v", len(res.Quotas), res.Quotas)
	}
	if res.Quotas["gemini"].DisplayName != "Gemini (Flash / Pro / Image / Search)" {
		t.Errorf("unexpected gemini label: %s", res.Quotas["gemini"].DisplayName)
	}
	if res.Quotas["claude"].DisplayName != "Claude & GPT (Shared)" {
		t.Errorf("unexpected claude label: %s", res.Quotas["claude"].DisplayName)
	}
}

func TestClusterAntigravitySummary(t *testing.T) {
	remSession := 0.8
	remWeekly := 0.6
	summary := AntigravityQuotaSummaryResponse{
		Groups: []struct {
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
			Buckets     []struct {
				BucketID          string   `json:"bucketId"`
				Window            string   `json:"window"`
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
				DisplayName       string   `json:"displayName"`
				Description       string   `json:"description"`
			} `json:"buckets"`
		}{
			{
				DisplayName: "Gemini Models",
				Buckets: []struct {
					BucketID          string   `json:"bucketId"`
					Window            string   `json:"window"`
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					DisplayName       string   `json:"displayName"`
					Description       string   `json:"description"`
				}{
					{
						BucketID:          "gemini-session",
						Window:            "5h",
						RemainingFraction: &remSession,
					},
					{
						BucketID:          "gemini-weekly",
						Window:            "7d",
						RemainingFraction: &remWeekly,
					},
				},
			},
			{
				DisplayName: "Claude and GPT models",
				Buckets: []struct {
					BucketID          string   `json:"bucketId"`
					Window            string   `json:"window"`
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
					DisplayName       string   `json:"displayName"`
					Description       string   `json:"description"`
				}{
					{
						BucketID:          "claude-gpt-session",
						Window:            "5h",
						RemainingFraction: &remSession,
					},
					{
						BucketID:          "claude-gpt-weekly",
						Window:            "7d",
						RemainingFraction: &remWeekly,
					},
				},
			},
		},
	}

	quotas := clusterAntigravitySummary(summary)
	if len(quotas) != 4 {
		t.Fatalf("expected 4 summary quota buckets, got %d: %+v", len(quotas), quotas)
	}

	gSess, ok := quotas["gemini_session"]
	if !ok || gSess.DisplayName != "Gemini (Search / Image) (5h Rolling)" || gSess.RemainingPercentage != 80.0 {
		t.Errorf("unexpected gemini_session: %+v", gSess)
	}

	gWeek, ok := quotas["gemini_weekly"]
	if !ok || gWeek.DisplayName != "Gemini (Search / Image) (Weekly)" || gWeek.RemainingPercentage != 60.0 {
		t.Errorf("unexpected gemini_weekly: %+v", gWeek)
	}

	cSess, ok := quotas["claude_session"]
	if !ok || cSess.DisplayName != "Claude & GPT (5h Rolling)" || cSess.RemainingPercentage != 80.0 {
		t.Errorf("unexpected claude_session: %+v", cSess)
	}

	cWeek, ok := quotas["claude_weekly"]
	if !ok || cWeek.DisplayName != "Claude & GPT (Weekly)" || cWeek.RemainingPercentage != 60.0 {
		t.Errorf("unexpected claude_weekly: %+v", cWeek)
	}
}
