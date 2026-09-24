package db

import (
	"errors"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestConnectionRuntimePartialUpdates(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	conn := &model.ProviderConnection{ID: "partial", Provider: "openai", AuthType: "oauth", Name: "admin", Priority: 9, IsActive: false, Data: model.ConnectionData{APIKey: "admin-key", BaseURL: "https://admin.example", AccessToken: "old-access", RefreshToken: "old-refresh", TestStatus: "error", LastError: "old error", RateLimitedUntil: "later", BackoffLevel: 5, ProviderSpecificData: map[string]any{"proxyPoolId": "admin-pool", "nested": map[string]any{"keep": "admin", "project": "old"}}}}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}
	// Non-health fields are deliberately stale and must never be persisted.
	stale := model.ConnectionData{APIKey: "stale-key", BaseURL: "https://stale.example", TestStatus: "active", ProviderSpecificData: map[string]any{"proxyPoolId": "stale-pool"}}
	if err := database.UpdateConnectionHealth(conn.ID, stale); err != nil {
		t.Fatal(err)
	}
	got, err := database.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.TestStatus != "active" || got.Data.LastError != "" || got.Data.RateLimitedUntil != "" || got.Data.BackoffLevel != 0 {
		t.Errorf("zero-valued health fields not cleared: %+v", got.Data)
	}
	stale.AccessToken, stale.RefreshToken, stale.ExpiresAt = "new-access", "new-refresh", "new-expiry"
	if err := database.UpdateConnectionTokens(conn.ID, "old-access", "old-refresh", stale, map[string]any{"nested": map[string]any{"project": "new"}}); err != nil {
		t.Fatal(err)
	}
	if err := database.UpdateConnectionProviderData(conn.ID, map[string]any{"projectId": "discovered"}); err != nil {
		t.Fatal(err)
	}
	got, err = database.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsActive || got.Name != "admin" || got.Priority != 9 || got.Data.APIKey != "admin-key" || got.Data.BaseURL != "https://admin.example" || got.Data.ProviderSpecificData["proxyPoolId"] != "admin-pool" {
		t.Errorf("partial update overwrote configuration: %+v", got)
	}
	if got.Data.AccessToken != "new-access" || got.Data.RefreshToken != "new-refresh" || got.Data.ExpiresAt != "new-expiry" || got.Data.TestStatus != "active" {
		t.Errorf("token update failed or overwrote health: %+v", got.Data)
	}
	nested, ok := got.Data.ProviderSpecificData["nested"].(map[string]any)
	if !ok || nested["keep"] != "admin" || nested["project"] != "new" || got.Data.ProviderSpecificData["projectId"] != "discovered" {
		t.Errorf("provider data was not incrementally merged: %+v", got.Data.ProviderSpecificData)
	}
	// Either token changing must reject the entire refresh patch.
	for _, tokens := range [][2]string{{"old-access", "new-refresh"}, {"new-access", "old-refresh"}} {
		if err := database.UpdateConnectionTokens(conn.ID, tokens[0], tokens[1], stale, map[string]any{"projectId": "stale"}); err == nil {
			t.Error("stale credential refresh succeeded")
		}
	}
	got, err = database.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.ProviderSpecificData["projectId"] != "discovered" {
		t.Error("rejected refresh still changed provider data")
	}
	if err := database.UpdateConnectionTokens(conn.ID, "new-access", "new-refresh", model.ConnectionData{}, nil); err != nil {
		t.Fatal(err)
	}
	got, err = database.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.AccessToken != "" || got.Data.RefreshToken != "" || got.Data.ExpiresAt != "" || got.Data.ProviderSpecificData["projectId"] != "discovered" {
		t.Error("empty token update did not clear tokens or nil patch erased provider data")
	}
	for _, update := range []func() error{
		func() error { return database.UpdateConnectionHealth("missing", stale) },
		func() error { return database.UpdateConnectionTokens("missing", "", "", stale, nil) },
		func() error {
			return database.UpdateConnectionProviderData("missing", map[string]any{"projectId": "value"})
		},
	} {
		if err := update(); !errors.Is(err, ErrNotFound) {
			t.Errorf("missing row error = %v, want ErrNotFound", err)
		}
	}
}
