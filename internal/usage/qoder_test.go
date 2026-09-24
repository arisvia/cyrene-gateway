package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Device/job tokens hit the quota endpoint directly (no PAT exchange).
func TestFetchQoder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/quota/usage" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer jt-test-token" {
			t.Errorf("expected Bearer jt-test-token, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userQuota": {"total": 2000, "used": 500, "remaining": 1500, "unit": "credits"},
			"orgResourcePackage": {"total": 0, "used": 0, "remaining": 0},
			"totalUsagePercentage": 25,
			"isQuotaExceeded": false,
			"expiresAt": 1781594470000
		}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
	})
	if !strings.Contains(res.Plan, "25%") {
		t.Errorf("expected plan with 25%% usage, got %q", res.Plan)
	}
	q, ok := res.Quotas["user"]
	if !ok {
		t.Fatalf("expected user quota, got %v", res.Quotas)
	}
	if q.Used != 500 || q.Total != 2000 || q.Remaining != 1500 {
		t.Errorf("unexpected quota values: %+v", q)
	}
	if q.ResetAt == "" {
		t.Error("expected resetAt from expiresAt")
	}
	if _, ok := res.Quotas["organization"]; ok {
		t.Error("zero-value org quota should be omitted")
	}
}
func TestFetchQoder_FractionUsagePercentage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userQuota": {"total": 3000, "used": 952, "remaining": 2048, "unit": "credits"},
			"orgResourcePackage": {"total": 2000, "used": 0, "remaining": 2000, "unit": "credits"},
			"totalUsagePercentage": 0.32,
			"isQuotaExceeded": false,
			"expiresAt": 1781594470000
		}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
	})
	if !strings.Contains(res.Plan, "32%") {
		t.Errorf("expected plan with 32%% usage, got %q", res.Plan)
	}
}

func TestFetchQoderNoToken(t *testing.T) {
	res := FetchQuota(context.Background(), nil, QuotaCredentials{Provider: "qoder"})
	if !strings.Contains(res.Message, "no access token") {
		t.Errorf("expected no-token message, got %q", res.Message)
	}
}
func TestFetchQoder_AddOnQuotaAndDynamicBuckets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userId": "019fe95f-b50b-745f-8fd9-5c917f5998ea",
			"userType": "personal_standard",
			"usageType": "credits",
			"totalUsagePercentage": 0,
			"isQuotaExceeded": false,
			"expiresAt": 253402214400000,
			"userQuota": {"total": 0, "used": 0, "remaining": 0, "percentage": 0, "unit": "credits"},
			"addOnQuota": {
				"total": 100,
				"used": 0,
				"remaining": 100,
				"percentage": 0,
				"unit": "credits",
				"detailUrl": "https://qoder.com/account/usage"
			},
			"campaignQuota": {
				"total": 500,
				"used": 50,
				"remaining": 450,
				"unit": "credits"
			}
		}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
	})

	if !strings.Contains(res.Plan, "Personal Standard") {
		t.Errorf("expected plan to mention Personal Standard, got %q", res.Plan)
	}

	addOn, ok := res.Quotas["addOn"]
	if !ok {
		t.Fatalf("expected addOn quota to be unpacked, got %v", res.Quotas)
	}
	if addOn.Total != 100 || addOn.Remaining != 100 || addOn.RemainingPercentage != 100 || addOn.DisplayName != "Add-on Credits" {
		t.Errorf("unexpected addOn values: %+v", addOn)
	}
	if addOn.ResetAt != "" {
		t.Errorf("expected perpetual timestamp 253402214400000 to be ignored, got %q", addOn.ResetAt)
	}

	campaign, ok := res.Quotas["campaignQuota"]
	if !ok {
		t.Fatalf("expected campaignQuota to be dynamically unpacked, got %v", res.Quotas)
	}
	if campaign.Total != 500 || campaign.Remaining != 450 || campaign.DisplayName != "Campaign Quota" {
		t.Errorf("unexpected campaign quota: %+v", campaign)
	}
}
func TestFetchQoder_DisplayLabelsAndTeamsPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userId": "019e2443-99d9-7d40-8248-d5da53fb0cb5",
			"userType": "teams",
			"usageType": "credits",
			"totalUsagePercentage": 0,
			"isQuotaExceeded": true,
			"expiresAt": 1790230166296,
			"userQuota": {"total": 0, "used": 0, "remaining": 0, "percentage": 0, "unit": "credits"},
			"dedicatedResourcePackages": [
				{
					"id": "01a0d1fb-5b2b-79b0-a014-c935b8d509c9",
					"name": "act-20260923-154",
					"description": "growth-campaign:act-20260923-154:grant:01a0d1fb-5b25-7a31-8ef6-02facff2e4e4",
					"total": 4000,
					"used": 0,
					"remaining": 4000,
					"percentage": 0,
					"unit": "credits",
					"expiresAt": 1790310599000,
					"displayLabels": [
						{
							"dimension": "description",
							"value": "sota model series description",
							"valueI18n": {
								"en-US": "SOTA Exclusive Credits: used first when you select the Sonus model.",
								"zh-CN": "SOTA 专属积分：在模型选择器中选择 Sonus 模型时优先抵扣该积分。"
							}
						},
						{
							"dimension": "title",
							"value": "sota model series",
							"valueI18n": {
								"en-US": "SOTA Exclusive Credits",
								"zh-CN": "SOTA 专属积分"
							}
						}
					]
				}
			],
			"orgResourcePackage": {
				"used": 0,
				"remaining": 2000,
				"percentage": 0,
				"unit": "credits",
				"cap": 2000,
				"available": true
			}
		}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider:     "qoder",
		AccessToken:  "jt-test-token",
		BaseURL:      srv.URL,
		Organization: "麦穗通信",
	})

	if res.Plan != "Qoder (Teams)" {
		t.Errorf("expected plan to remain Teams when remaining quota exists, got %q", res.Plan)
	}

	pkg, ok := res.Quotas["dedicatedResourcePackages_1"]
	if !ok {
		t.Fatalf("expected dedicated package to be extracted, got %v", res.Quotas)
	}
	if pkg.DisplayName != "SOTA 专属积分" {
		t.Errorf("expected DisplayName to be SOTA 专属积分 from displayLabels, got %q", pkg.DisplayName)
	}
	if pkg.Remaining != 4000 || pkg.Total != 4000 {
		t.Errorf("unexpected dedicated package values: %+v", pkg)
	}

	org, ok := res.Quotas["organization"]
	if !ok {
		t.Fatalf("expected organization quota to be extracted, got %v", res.Quotas)
	}
	if org.DisplayName != "麦穗通信" {
		t.Errorf("expected organization DisplayName to be 麦穗通信, got %q", org.DisplayName)
	}
}
