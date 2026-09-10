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
			// Image model
			"gemini-3.1-flash-image": {
				DisplayName: "Gemini 3.1 Flash Image",
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

	// Must cluster exactly into the 4 buckets matching 9router UI
	if len(quotas) != 4 {
		t.Fatalf("expected exactly 4 clustered quota buckets, got %d: %+v", len(quotas), quotas)
	}

	geminiQ, ok := quotas["gemini"]
	if !ok {
		t.Fatalf("missing 'gemini' quota")
	}
	if geminiQ.DisplayName != "Gemini (Flash / Pro)" {
		t.Errorf("expected 'Gemini (Flash / Pro)', got %q", geminiQ.DisplayName)
	}
	if geminiQ.Used != 50 || geminiQ.Remaining != 950 || geminiQ.RemainingPercentage != 95.0 {
		t.Errorf("unexpected gemini quota math: %+v", geminiQ)
	}

	claudeQ, ok := quotas["claude"]
	if !ok {
		t.Fatalf("missing 'claude' quota")
	}
	if claudeQ.DisplayName != "Claude (Sonnet / Opus)" {
		t.Errorf("expected 'Claude (Sonnet / Opus)', got %q", claudeQ.DisplayName)
	}
	if claudeQ.Used != 0 || claudeQ.Remaining != 1000 || claudeQ.RemainingPercentage != 100.0 {
		t.Errorf("unexpected claude quota math: %+v", claudeQ)
	}

	gptQ, ok := quotas["gpt-oss-120b-medium"]
	if !ok {
		t.Fatalf("missing 'gpt-oss-120b-medium' quota")
	}
	if gptQ.DisplayName != "GPT-OSS 120B (Medium)" {
		t.Errorf("expected 'GPT-OSS 120B (Medium)', got %q", gptQ.DisplayName)
	}

	imgQ, ok := quotas["gemini-3.1-flash-image"]
	if !ok {
		t.Fatalf("missing 'gemini-3.1-flash-image' quota")
	}
	if imgQ.DisplayName != "Gemini 3.1 Flash Image" {
		t.Errorf("expected 'Gemini 3.1 Flash Image', got %q", imgQ.DisplayName)
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
	if res.Quotas["gemini"].DisplayName != "Gemini (Flash / Pro)" {
		t.Errorf("unexpected gemini label: %s", res.Quotas["gemini"].DisplayName)
	}
	if res.Quotas["claude"].DisplayName != "Claude (Sonnet / Opus)" {
		t.Errorf("unexpected claude label: %s", res.Quotas["claude"].DisplayName)
	}
}
