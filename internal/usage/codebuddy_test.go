package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchCodebuddy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer cb-test-token" {
			t.Errorf("expected Bearer cb-test-token, got %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "CLI/2.108.1 CodeBuddy/2.108.1" {
			t.Errorf("unexpected User-Agent: %q", got)
		}
		if got := r.Header.Get("x-codebuddy-request"); got != "1" {
			t.Errorf("expected x-codebuddy-request: 1, got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"code": 0,
			"msg": "OK",
			"data": {
				"Response": {
					"Data": {
						"Accounts": [
							{
								"AccountId": 1,
								"PackageName": "CodeBuddy个人体验版",
								"CapacityUnit": "credits",
								"CycleStartTime": "2026-09-01 00:00:00",
								"CycleEndTime": "2026-09-30 23:59:59",
								"DeductionEndTime": 2035447069000,
								"CycleCapacitySizePrecise": "500",
								"CycleCapacityUsedPrecise": "358.5",
								"CycleCapacityRemainPrecise": "141.5",
								"Status": 0
							},
							{
								"AccountId": 2,
								"PackageName": "CodeBuddy赠送包",
								"CapacityUnit": "credits",
								"CycleEndTime": "2027-04-02 17:57:50",
								"DeductionEndTime": 1806659870000,
								"CapacitySizePrecise": "3000",
								"CapacityUsedPrecise": "0",
								"CapacityRemainPrecise": "3000",
								"Status": 0
							},
							{
								"AccountId": 3,
								"PackageName": "已过期裂变包",
								"CapacityUnit": "credits",
								"CycleEndTime": "2026-07-01 00:00:00",
								"DeductionEndTime": 1782800000000,
								"CapacitySizePrecise": "100",
								"CapacityUsedPrecise": "100",
								"CapacityRemainPrecise": "0",
								"Status": 3
							}
						]
					}
				}
			}
		}`))
	}))
	defer srv.Close()

	res := FetchQuota(context.Background(), srv.Client(), QuotaCredentials{
		Provider:    "codebuddy-cn",
		AccessToken: "cb-test-token",
		BaseURL:     srv.URL,
	})

	if res.Plan != "CodeBuddy个人体验版" {
		t.Errorf("unexpected plan: %q", res.Plan)
	}

	monthly, ok := res.Quotas["Monthly"]
	if !ok {
		t.Fatalf("expected 'Monthly' quota bucket, got %v", res.Quotas)
	}
	if monthly.Total != 500 || monthly.Used != 358.5 || monthly.Remaining != 141.5 {
		t.Errorf("unexpected monthly quota: %+v", monthly)
	}
	if monthly.Unit != "credits" {
		t.Errorf("expected unit 'credits', got %q", monthly.Unit)
	}
	if monthly.ResetAt == "" {
		t.Error("expected non-empty resetAt for Monthly pack")
	}

	bonus, ok := res.Quotas["Bonus Pack 1"]
	if !ok {
		t.Fatalf("expected 'Bonus Pack 1' quota bucket, got %v", res.Quotas)
	}
	if bonus.Total != 3000 || bonus.Remaining != 3000 || bonus.Used != 0 {
		t.Errorf("unexpected bonus pack quota: %+v", bonus)
	}

	if _, ok := res.Quotas["Bonus Pack 2"]; ok {
		t.Error("expired zero-balance account should be excluded")
	}
}

func TestFetchCodebuddyNoToken(t *testing.T) {
	res := FetchQuota(context.Background(), nil, QuotaCredentials{Provider: "codebuddy-cn"})
	if res.Message == "" {
		t.Error("expected error message when no token provided")
	}
}
