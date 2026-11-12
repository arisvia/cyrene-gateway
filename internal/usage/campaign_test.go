package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCampaignSupportedRegistry(t *testing.T) {
	for _, id := range []string{"qoder", "qoder-cn", "codebuddy", "codebuddy-cn"} {
		if !CampaignSupported(id) {
			t.Errorf("%s must expose a campaign adapter", id)
		}
	}
	for _, id := range []string{"openai", "anthropic", "antigravity"} {
		if CampaignSupported(id) {
			t.Errorf("%s must not claim campaign support", id)
		}
	}
}

// newQoderCampaignMock serves the /sash campaigns surface the desktop client
// sees: the list is only populated for Cosy-ClientType=10 with a device id,
// claims are idempotent, and non-benefit tasks must be filtered out.
func newQoderCampaignMock(t *testing.T) (*httptest.Server, map[string]string) {
	claimStatus := map[string]string{"camp-daily": "CLAIMABLE", "camp-old": "CLAIMED"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sash/api/v1/me/campaigns" {
			if got := r.Header.Get("Cosy-ClientType"); got != "10" {
				t.Errorf("campaigns list requires Cosy-ClientType=10, got %q", got)
			}
			if got := r.Header.Get("Cosy-MachineId"); got != "machine-1" {
				t.Errorf("campaign list gated on device identity, got machine %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer dt-token" {
				t.Errorf("unexpected auth: %q", got)
			}
			items := []map[string]any{}
			for _, id := range []string{"camp-daily", "camp-old"} {
				amount := 100.0
				if id == "camp-old" {
					amount = 50
				}
				items = append(items, map[string]any{
					"campaignId":  id,
					"campaignKey": id,
					"actionType":  "CLAIM_BENEFIT",
					"claimStatus": claimStatus[id],
					"benefit":     map[string]any{"amount": amount},
					"endAt":       1_790_000_000,
				})
			}
			items = append(items, map[string]any{
				"campaignId": "task-signup", "actionType": "COMPLETE_TASK", "claimStatus": "CLAIMABLE",
			})
			_ = json.NewEncoder(w).Encode(map[string]any{"campaigns": items})
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sash/api/v1/me/campaigns/") && strings.HasSuffix(r.URL.Path, "/claim") {
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/sash/api/v1/me/campaigns/"), "/claim")
			claimStatus[id] = "CLAIMED"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "CLAIMED", "replayed": false, "benefit": map[string]any{"amount": 100}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	return ts, claimStatus
}

func TestQoderCampaignClaimFlow(t *testing.T) {
	ts, claimStatus := newQoderCampaignMock(t)
	creds := QuotaCredentials{Provider: "qoder", AccessToken: "dt-token", MachineID: "machine-1", BaseURL: ts.URL}

	res := ClaimCampaigns(context.Background(), ts.Client(), creds)
	if !res.Supported {
		t.Fatal("qoder must be campaign-supported")
	}
	if res.Outcome != ClaimOutcomeClaimed || res.Count != 1 || res.Amount != 100 {
		t.Fatalf("first claim = %+v, want claimed count=1 amount=100", res)
	}
	if claimStatus["camp-daily"] != "CLAIMED" {
		t.Fatal("upstream claim status not advanced")
	}
	if claimStatus["task-signup"] == "CLAIMED" {
		t.Fatal("non-benefit task must never be claimed")
	}

	// Second click: everything is CLAIMED upstream → idempotent "already".
	again := ClaimCampaigns(context.Background(), ts.Client(), creds)
	if again.Outcome != ClaimOutcomeAlready || again.Count != 0 {
		t.Fatalf("second claim = %+v, want already count=0", again)
	}
}

func TestQoderCampaignTokenFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()
	res := ClaimCampaigns(context.Background(), ts.Client(), QuotaCredentials{
		Provider: "qoder-cn", AccessToken: "dt-expired", BaseURL: ts.URL,
	})
	if res.Outcome != ClaimOutcomeError || !strings.Contains(res.Message, "unauthorized") {
		t.Fatalf("expired token = %+v, want error mentioning unauthorized", res)
	}
}

// newCodebuddyCheckinMock serves the Buddy gas-station surface:
// checkin-activity-status + daily-checkin, with business errors in the
// upstream's own wording ("今天已签到" on repeats).
func newCodebuddyCheckinMock(t *testing.T) *httptest.Server {
	active, checkedIn := true, false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("x-codebuddy-request") != "1" {
			t.Errorf("billing meter requires the codebuddy plugin marker header")
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("billing meter requires bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/billing/meter/checkin-activity-status":
			if !active {
				fmt.Fprint(w, `{"code":0,"msg":"OK","data":{"active":false}}`)
				return
			}
			fmt.Fprintf(w, `{"code":0,"msg":"OK","data":{"active":true,"today_checked_in":%t,"today_credit":100,"streak_days":3,"end_time":"2026-12-31 23:59:59"}}`, checkedIn)
		case "/v2/billing/meter/daily-checkin":
			if checkedIn {
				fmt.Fprint(w, `{"code":-1,"msg":"今天已签到，请明天再来"}`)
				return
			}
			checkedIn = true
			fmt.Fprint(w, `{"code":0,"msg":"OK","data":{"credit":100}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(func() { ts.Close() })
	return ts
}

func TestCodebuddyCheckinClaimFlow(t *testing.T) {
	ts := newCodebuddyCheckinMock(t)
	creds := QuotaCredentials{Provider: "codebuddy-cn", AccessToken: "jwt-token", BaseURL: ts.URL}

	res := ClaimCampaigns(context.Background(), ts.Client(), creds)
	if res.Outcome != ClaimOutcomeClaimed || res.Count != 1 || res.Amount != 100 {
		t.Fatalf("first check-in = %+v, want claimed count=1 amount=100", res)
	}

	// Repeated click: status already says checked in → "already" without POST.
	again := ClaimCampaigns(context.Background(), ts.Client(), creds)
	if again.Outcome != ClaimOutcomeAlready {
		t.Fatalf("second check-in = %+v, want already", again)
	}

}

// Race window: the status poll says unchecked, but the claim itself answers
// with the upstream's "今天已签到" business error (a competing claim landed in
// between). That must surface as already, not error.
func TestCodebuddyCheckinClaimRace(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/billing/meter/checkin-activity-status" {
			fmt.Fprint(w, `{"code":0,"msg":"OK","data":{"active":true,"today_checked_in":false,"today_credit":100}}`)
			return
		}
		fmt.Fprint(w, `{"code":-1,"msg":"今天已签到，请明天再来"}`)
	}))
	defer ts.Close()
	res := ClaimCampaigns(context.Background(), ts.Client(), QuotaCredentials{
		Provider: "codebuddy-cn", AccessToken: "jwt-token", BaseURL: ts.URL,
	})
	if res.Outcome != ClaimOutcomeAlready {
		t.Fatalf("raced check-in = %+v, want already", res)
	}
}

func TestCodebuddyCheckinInactive(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"msg":"OK","data":{"active":false}}`)
	}))
	defer ts.Close()
	res := ClaimCampaigns(context.Background(), ts.Client(), QuotaCredentials{
		Provider: "codebuddy", AccessToken: "jwt-token", BaseURL: ts.URL,
	})
	if res.Outcome != ClaimOutcomeNone {
		t.Fatalf("inactive activity = %+v, want none", res)
	}
}

func TestCodebuddyCheckinUnsupportedHost(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	res := ClaimCampaigns(context.Background(), ts.Client(), QuotaCredentials{
		Provider: "codebuddy", AccessToken: "jwt-token", BaseURL: ts.URL,
	})
	if res.Outcome != ClaimOutcomeError {
		t.Fatalf("404 activity API = %+v, want error", res)
	}
}
