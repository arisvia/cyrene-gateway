package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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
		for header, want := range map[string]string{
			"User-Agent": "qodercli/1.1.80", "Cosy-Version": "1.26.0", "Cosy-ClientType": "5", "Accept": "application/json",
		} {
			if got := r.Header.Get(header); got != want {
				t.Errorf("expected %s: %q, got %q", header, want, got)
			}
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

func TestFetchQoder_BucketPercentageUsesBalances(t *testing.T) {
	for _, tt := range []struct {
		name          string
		bucket        string
		wantRemaining float64
		wantPct       float64
	}{
		{
			name: "one_percent_used", bucket: `{"total":100,"used":1,"remaining":99,"percentage":1}`,
			wantRemaining: 99, wantPct: 99,
		},
		{
			name: "half_percent_used", bucket: `{"total":100,"used":0.5,"remaining":99.5,"percentage":0.5}`,
			wantRemaining: 99.5, wantPct: 99.5,
		},
		{
			name: "explicit_zero_remaining", bucket: `{"total":100,"used":1,"remaining":0,"percentage":0.5}`,
			wantRemaining: 0, wantPct: 0,
		},
		{
			name: "absent_remaining", bucket: `{"total":100,"used":1,"percentage":1}`,
			wantRemaining: 99, wantPct: 99,
		},
		{
			name: "absent_remaining_zero_used", bucket: `{"total":100,"used":0,"percentage":0.5}`,
			wantRemaining: 100, wantPct: 100,
		},
		{
			name: "cap_with_absent_remaining", bucket: `{"cap":100,"used":0.5,"percentage":0.5}`,
			wantRemaining: 99.5, wantPct: 99.5,
		},
		{
			name: "fraction_percentage_fallback", bucket: `{"total":100,"percentage":0.5}`,
			wantRemaining: 0, wantPct: 50,
		},
		{
			name: "whole_percentage_fallback", bucket: `{"total":100,"percentage":25}`,
			wantRemaining: 0, wantPct: 75,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"userQuota":` + tt.bucket + `}`))
			}))
			defer srv.Close()

			res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
				Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
			})
			q, ok := res.Quotas["user"]
			if !ok {
				t.Fatalf("expected user quota, got %+v", res)
			}
			if q.Remaining != tt.wantRemaining || q.RemainingPercentage != tt.wantPct {
				t.Errorf("remaining = %v (%v%%), want %v (%v%%)", q.Remaining, q.RemainingPercentage, tt.wantRemaining, tt.wantPct)
			}
		})
	}
}

func TestFetchQoder_ExceededUsesGenerallyAvailableBalances(t *testing.T) {
	for _, tt := range []struct {
		name          string
		buckets       string
		wantPlan      string
		wantRemaining map[string]float64
	}{
		{
			name: "dedicated_only_positive",
			buckets: `"userQuota":{"total":100,"used":100,"remaining":0},
				"dedicatedResourcePackages":[{"total":4000,"used":0,"remaining":4000}]`,
			wantPlan:      "Qoder (Quota Exceeded)",
			wantRemaining: map[string]float64{"user": 0, "dedicatedResourcePackages_1": 4000},
		},
		{
			name:          "unavailable_organization",
			buckets:       `"orgResourcePackage":{"cap":2000,"used":0,"remaining":2000,"available":false}`,
			wantPlan:      "Qoder (Quota Exceeded)",
			wantRemaining: map[string]float64{"organization": 2000},
		},
		{
			name:          "available_organization",
			buckets:       `"orgResourcePackage":{"cap":2000,"used":0,"remaining":2000,"available":true}`,
			wantPlan:      "Qoder (Teams)",
			wantRemaining: map[string]float64{"organization": 2000},
		},
		{
			name:          "organization_availability_unspecified",
			buckets:       `"orgResourcePackage":{"cap":2000,"used":0,"remaining":2000}`,
			wantPlan:      "Qoder (Teams)",
			wantRemaining: map[string]float64{"organization": 2000},
		},
		{
			name: "exhausted_global_balances",
			buckets: `"userQuota":{"total":100,"used":100,"remaining":0},
				"addOnQuota":{"total":100,"used":100,"remaining":0},
				"orgResourcePackage":{"cap":2000,"used":2000,"remaining":0,"available":true}`,
			wantPlan:      "Qoder (Quota Exceeded)",
			wantRemaining: map[string]float64{"user": 0, "addOn": 0, "organization": 0},
		},
		{
			name:          "available_user",
			buckets:       `"userQuota":{"total":100,"used":1,"remaining":99}`,
			wantPlan:      "Qoder (Teams)",
			wantRemaining: map[string]float64{"user": 99},
		},
		{
			name:          "available_add_on",
			buckets:       `"addOnQuota":{"total":100,"used":1,"remaining":99}`,
			wantPlan:      "Qoder (Teams)",
			wantRemaining: map[string]float64{"addOn": 99},
		},
		{
			name:          "unrecognized_promotional_balance",
			buckets:       `"campaignQuota":{"total":500,"used":50,"remaining":450}`,
			wantPlan:      "Qoder (Quota Exceeded)",
			wantRemaining: map[string]float64{"campaignQuota": 450},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"userType":"teams","isQuotaExceeded":true,` + tt.buckets + `}`))
			}))
			defer srv.Close()

			res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
				Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
			})
			if res.Plan != tt.wantPlan {
				t.Errorf("plan = %q, want %q", res.Plan, tt.wantPlan)
			}
			for key, want := range tt.wantRemaining {
				if q, ok := res.Quotas[key]; !ok || q.Remaining != want {
					t.Errorf("quota %q = %+v (present: %v), want remaining %v", key, q, ok, want)
				}
			}
		})
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
	if addOn.Total != 100 || addOn.Remaining != 100 || addOn.RemainingPercentage != 100 || addOn.DisplayName != "" {
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
	assertQoderLabelJSON(t, pkg, "sota model series", map[string]string{
		"en-US": "SOTA Exclusive Credits",
		"zh-CN": "SOTA 专属积分",
	})
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

func assertQoderLabelJSON(t *testing.T, q Quota, wantName string, wantI18n map[string]string) {
	t.Helper()
	raw, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		DisplayName     string            `json:"displayName"`
		DisplayNameI18n map[string]string `json:"displayNameI18n"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.DisplayName != wantName || !reflect.DeepEqual(payload.DisplayNameI18n, wantI18n) {
		t.Errorf("serialized label = %s, want displayName %q and translations %v", raw, wantName, wantI18n)
	}
	if wantI18n == nil && strings.Contains(string(raw), `"displayNameI18n"`) {
		t.Errorf("absent translations should be omitted: %s", raw)
	}
}

func TestFetchQoder_StandardBucketLabels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"userQuota":{"total":100,"remaining":100},
			"orgResourcePackage":{"cap":100,"remaining":100},
			"addOnQuota":{"total":100,"remaining":100}
		}`))
	}))
	defer srv.Close()

	for _, tt := range []struct {
		organization string
		wantName     string
	}{
		{organization: "   ", wantName: ""},
		{organization: "  麦穗通信  ", wantName: "麦穗通信"},
	} {
		t.Run(tt.organization, func(t *testing.T) {
			res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
				Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL, Organization: tt.organization,
			})
			for key, want := range map[string]string{"user": "", "organization": tt.wantName, "addOn": ""} {
				q, ok := res.Quotas[key]
				if !ok {
					t.Fatalf("missing quota %q: %+v", key, res)
				}
				assertQoderLabelJSON(t, q, want, nil)
			}
		})
	}
}

func TestFetchQoder_LocalizedLabels(t *testing.T) {
	for _, tt := range []struct {
		name     string
		fields   string
		wantName string
		wantI18n map[string]string
	}{
		{
			name: "camel_title_before_description",
			fields: `"displayLabels":[
				{"dimension":"description","value":"Description","valueI18n":{"en-US":"Details","zh-CN":"说明"}},
				{"dimension":"title","value":"  Raw title  ","valueI18n":{"en-US":"  Exclusive credits  ","zh-CN":" 专属积分 ","fr-FR":"Crédits exclusifs","empty":"  ","invalid":42}}
			],"name":"act-internal"`,
			wantName: "Raw title", wantI18n: map[string]string{"en-US": "Exclusive credits", "zh-CN": "专属积分", "fr-FR": "Crédits exclusifs"},
		},
		{
			name: "snake_name_before_description",
			fields: `"display_labels":[
				{"dimension":"description","value":"Description","value_i18n":{"en-US":"Details"}},
				{"dimension":"name","value":"Raw name","value_i18n":{"en-US":"Named credits","zh-CN":"命名积分"}}
			]`,
			wantName: "Raw name", wantI18n: map[string]string{"en-US": "Named credits", "zh-CN": "命名积分"},
		},
		{
			name:     "camel_description",
			fields:   `"displayLabels":[{"dimension":"description","value":"Raw description","valueI18n":{"en-US":"Credit details","zh-CN":"积分说明"}}]`,
			wantName: "Raw description", wantI18n: map[string]string{"en-US": "Credit details", "zh-CN": "积分说明"},
		},
		{
			name:     "snake_description_without_raw_value",
			fields:   `"display_labels":[{"dimension":"description","value_i18n":{"en-US":"Credit details","zh-CN":"积分说明"}}],"title":"  Custom title  "`,
			wantName: "Custom title", wantI18n: map[string]string{"en-US": "Credit details", "zh-CN": "积分说明"},
		},
		{
			name: "localized_title_keeps_priority_without_raw_value",
			fields: `"displayLabels":[
				{"dimension":"description","value":"Description","valueI18n":{"en-US":"Details"}},
				{"dimension":"title","valueI18n":{"zh-CN":"专属积分"}}
			],"displayName":"Custom display name"`,
			wantName: "Custom display name", wantI18n: map[string]string{"zh-CN": "专属积分"},
		},
		{
			name:     "internal_label_value_uses_package_name",
			fields:   `"displayLabels":[{"dimension":"title","value":"pkg-internal","valueI18n":{"en-US":"Exclusive credits"}}],"packageName":"Custom package"`,
			wantName: "Custom package", wantI18n: map[string]string{"en-US": "Exclusive credits"},
		},
		{name: "raw_title", fields: `"title":"Custom title","displayName":"Lower priority"`, wantName: "Custom title"},
		{name: "raw_display_name", fields: `"displayName":"Custom display name"`, wantName: "Custom display name"},
		{name: "raw_package_name", fields: `"packageName":"Custom package"`, wantName: "Custom package"},
		{name: "custom_name", fields: `"name":"  My credits  "`, wantName: "My credits"},
		{name: "internal_act_name", fields: `"name":"act-internal"`},
		{name: "internal_growth_name", fields: `"name":"growth-internal"`},
		{name: "internal_package_name", fields: `"name":"pkg-internal"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"userQuota":{"total":100,"remaining":100,` + tt.fields + `}}`))
			}))
			defer srv.Close()
			res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
				Provider: "qoder", AccessToken: "jt-test-token", BaseURL: srv.URL,
			})
			q, ok := res.Quotas["user"]
			if !ok {
				t.Fatalf("missing user quota: %+v", res)
			}
			assertQoderLabelJSON(t, q, tt.wantName, tt.wantI18n)
		})
	}
}
