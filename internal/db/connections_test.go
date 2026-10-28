package db

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestConnectionReadsNormalizeNullableColumns(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// name, email and priority are nullable columns, so rows written before those
	// fields were populated arrive as NULL and must read back as zero values.
	if _, err := database.conn.Exec(`INSERT INTO providerConnections
		(id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
		VALUES ('nulls', 'qoder', 'api-key', NULL, NULL, NULL, 0, '{}', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateConnection(&model.ProviderConnection{
		ID: "active", Provider: "qoder", AuthType: "oauth", Name: "Five", Email: "five@example.test",
		Priority: 5, IsActive: true, Data: model.ConnectionData{AccessToken: "access"},
	}); err != nil {
		t.Fatal(err)
	}

	list, err := database.ListConnections()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "nulls" || list[1].ID != "active" {
		t.Fatalf("expected NULL priority first, got %v", idsOf(list))
	}
	nulls := list[0]
	if nulls.Name != "" || nulls.Email != "" || nulls.Priority != 0 || nulls.IsActive {
		t.Errorf("NULL columns must normalize to zero values, got %+v", nulls)
	}
	if nulls.CreatedAt.Year() != 2026 || nulls.CreatedAt.Month() != time.January || nulls.CreatedAt.Day() != 1 {
		t.Errorf("expected createdAt parsed, got %v", nulls.CreatedAt)
	}
	if nulls.UpdatedAt.IsZero() {
		t.Error("expected updatedAt parsed")
	}
	if list[1].Priority != 5 || !list[1].IsActive || list[1].Data.AccessToken != "access" {
		t.Errorf("populated row must round-trip, got %+v", list[1])
	}

	active, err := database.ListConnectionsByProvider("qoder")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].ID != "active" {
		t.Errorf("expected only the active connection, got %v", idsOf(active))
	}

	single, err := database.GetConnection("nulls")
	if err != nil {
		t.Fatal(err)
	}
	if single.Name != "" || single.Priority != 0 || single.IsActive {
		t.Errorf("GetConnection must normalize NULL columns identically, got %+v", single)
	}

	if _, err := database.GetConnection("absent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	if _, err := database.conn.Exec(`UPDATE providerConnections SET data = 'not-json' WHERE id = 'active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.GetConnection("active"); err == nil {
		t.Error("expected a corrupt data blob to surface as an error")
	}
	if _, err := database.ListConnections(); err == nil {
		t.Error("expected a corrupt data blob to abort the list query")
	}
}

func idsOf(list []model.ProviderConnection) []string {
	ids := make([]string, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	return ids
}

func TestConnectionProfileCompareAndSwap(t *testing.T) {
	for _, field := range []string{"none", "name-only", "name-only-no-provider-data", "name", "email", "organizationName", "organization_name", "provider", "authType", "apiKey", "accessToken", "refreshToken", "deleted"} {
		t.Run(field, func(t *testing.T) {
			database, err := Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			original := &model.ProviderConnection{
				ID: "profile", Provider: "qoder-cn", AuthType: "oauth", IsActive: true,
				Data: model.ConnectionData{AccessToken: "access", RefreshToken: "refresh", APIKey: "key", ProviderSpecificData: map[string]any{"nested": map[string]any{"keep": "value"}}},
			}
			if err := database.CreateConnection(original); err != nil {
				t.Fatal(err)
			}
			admin, err := database.GetConnection(original.ID)
			if err != nil {
				t.Fatal(err)
			}
			admin.Priority, admin.IsActive, admin.Data.QuotaLimit = 99, false, 123
			admin.Data.BaseURL, admin.Data.TestStatus = "https://admin.example.test", "error"
			admin.Data.ProviderSpecificData["proxyPoolId"] = "admin-pool"
			switch field {
			case "name-only-no-provider-data":
				admin.Data.ProviderSpecificData = nil
			case "name":
				admin.Name = "admin"
			case "email":
				admin.Email = "admin@example.test"
			case "organizationName", "organization_name":
				admin.Data.ProviderSpecificData[field] = "Admin Org"
			case "provider":
				admin.Provider = "qoder"
			case "authType":
				admin.AuthType = "access_token"
			case "apiKey":
				admin.Data.APIKey = "replacement-key"
			case "accessToken":
				admin.Data.AccessToken = "replacement-access"
			case "refreshToken":
				admin.Data.RefreshToken = "replacement-refresh"
			}
			if err := database.UpdateConnection(admin); err != nil {
				t.Fatal(err)
			}
			if field == "deleted" {
				if err := database.DeleteConnection(original.ID); err != nil {
					t.Fatal(err)
				}
			}
			organization := "Profile Org"
			if field == "name-only" || field == "name-only-no-provider-data" {
				organization = ""
			}
			err = database.UpdateConnectionProfile(original, "Profile Name", "profile@example.test", organization)
			wantErr := ErrConnectionProfileChanged
			if field == "none" || field == "name-only" || field == "name-only-no-provider-data" {
				wantErr = nil
				admin.Name, admin.Email = "Profile Name", "profile@example.test"
				if organization != "" {
					admin.Data.ProviderSpecificData["organizationName"] = "Profile Org"
				}
			} else if field == "deleted" {
				wantErr = ErrNotFound
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("profile CAS error=%v, want %v", err, wantErr)
			}
			got, err := database.GetConnection(original.ID)
			if field == "deleted" {
				if !errors.Is(err, ErrNotFound) {
					t.Errorf("enrichment recreated deleted connection: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Provider != admin.Provider || got.AuthType != admin.AuthType || got.Name != admin.Name || got.Email != admin.Email || got.Priority != admin.Priority || got.IsActive != admin.IsActive || !reflect.DeepEqual(got.Data, admin.Data) {
				t.Errorf("profile CAS lost current connection state: got=%+v, want=%+v", got, admin)
			}
		})
	}
}

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
