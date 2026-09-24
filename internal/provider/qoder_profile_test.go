package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchQoderUserProfileFor_CN_MobileAndOrg(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                "01a0d264-e7e5-7c3d-933f-4b577bf324ef",
			"name":              "herta",
			"username":          "8b7d5a12-eaa2-422e-a2e7-cfa15967bd7b",
			"security_mobile":   "13713981921",
			"organization_id":   "org-123",
			"organization_name": "麦穗通信",
		})
	}))
	defer ts.Close()

	orig := QoderCnUserinfoURL
	defer func() { QoderCnUserinfoURL = orig }()
	QoderCnUserinfoURL = ts.URL

	profile := FetchQoderUserProfileFor("qoder-cn", "test-token", ts.Client())
	if profile == nil {
		t.Fatal("expected profile, got nil")
	}
	if profile.Name != "herta" {
		t.Errorf("expected Name herta, got %q", profile.Name)
	}
	if profile.EffectiveEmail() != "13713981921" {
		t.Errorf("expected EffectiveEmail 13713981921 from security_mobile, got %q", profile.EffectiveEmail())
	}
	if profile.OrganizationName != "麦穗通信" {
		t.Errorf("expected OrganizationName 麦穗通信, got %q", profile.OrganizationName)
	}

	name, email := FetchQoderUserInfoFor("qoder-cn", "test-token", ts.Client())
	if name != "herta" || email != "13713981921" {
		t.Errorf("expected (herta, 13713981921), got (%q, %q)", name, email)
	}
}
