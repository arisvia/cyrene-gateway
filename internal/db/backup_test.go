package db

import (
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestBackupAndRestore(t *testing.T) {
	d := setupTestDB(t)

	// 1. Seed some test data
	conn := &model.ProviderConnection{
		ID:       "conn-1",
		Provider: "openai",
		AuthType: "api-key",
		Name:     "Test OpenAI",
		Priority: 1,
		IsActive: true,
		Data: model.ConnectionData{
			APIKey:       "sk-secret-key-12345",
			AccessToken:  "access-token-abc",
			RefreshToken: "refresh-token-xyz",
			BaseURL:      "https://api.openai.com/v1",
		},
	}
	if err := d.CreateConnection(conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}

	key := &model.APIKey{
		ID:            "key-1",
		Key:           "cg-client-key-999",
		Name:          "Dev Client",
		IsActive:      true,
		AllowedModels: []string{"gpt-4o", "claude-3-5-sonnet"},
		RPM:           60,
	}
	if err := d.CreateAPIKey(key); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	combo := &model.Combo{
		ID:     "combo-1",
		Name:   "smart-fallback",
		Kind:   "fallback",
		Models: []string{"openai/gpt-4o", "anthropic/claude-3-5-sonnet"},
	}
	if err := d.CreateCombo(combo); err != nil {
		t.Fatalf("create combo: %v", err)
	}

	if err := d.KVSet("testScope", "hello", "world"); err != nil {
		t.Fatalf("set kv: %v", err)
	}

	// 2. Test export with secrets
	exportWithSecrets, err := d.ExportData(true, false)
	if err != nil {
		t.Fatalf("export with secrets: %v", err)
	}
	if !exportWithSecrets.IncludesSecrets {
		t.Fatalf("expected IncludesSecrets to be true")
	}
	if len(exportWithSecrets.Data.ProviderConnections) != 1 {
		t.Fatalf("expected 1 connection in export")
	}
	if exportWithSecrets.Data.ProviderConnections[0].Data.APIKey != "sk-secret-key-12345" {
		t.Fatalf("expected APIKey to be preserved")
	}
	if len(exportWithSecrets.Data.APIKeys) != 1 || exportWithSecrets.Data.APIKeys[0].Key != "cg-client-key-999" {
		t.Fatalf("expected APIKey key to be preserved")
	}

	// 3. Test export WITHOUT secrets (sanitized)
	exportSanitized, err := d.ExportData(false, false)
	if err != nil {
		t.Fatalf("export sanitized: %v", err)
	}
	if exportSanitized.IncludesSecrets {
		t.Fatalf("expected IncludesSecrets to be false")
	}
	if exportSanitized.Data.ProviderConnections[0].Data.APIKey != "" {
		t.Fatalf("expected APIKey to be empty in sanitized export")
	}
	if exportSanitized.Data.APIKeys[0].Key != "" {
		t.Fatalf("expected APIKey key to be empty in sanitized export")
	}

	// 4. Test that importing sanitized export preserves existing credentials!
	// Modify something non-sensitive in sanitized export (e.g. name)
	exportSanitized.Data.ProviderConnections[0].Name = "Renamed OpenAI"
	if err := d.ImportData(exportSanitized, RestoreModeMerge); err != nil {
		t.Fatalf("import sanitized in merge mode: %v", err)
	}

	cAfter, err := d.GetConnection("conn-1")
	if err != nil {
		t.Fatalf("get connection after import: %v", err)
	}
	if cAfter.Name != "Renamed OpenAI" {
		t.Fatalf("expected name to update to 'Renamed OpenAI', got %s", cAfter.Name)
	}
	if cAfter.Data.APIKey != "sk-secret-key-12345" {
		t.Fatalf("CRITICAL: sanitized import wiped existing APIKey! Expected sk-secret-key-12345, got %s", cAfter.Data.APIKey)
	}
	if cAfter.Data.AccessToken != "access-token-abc" {
		t.Fatalf("CRITICAL: sanitized import wiped existing AccessToken!")
	}

	// 5. Test restore to a fresh database
	dFresh := setupTestDB(t)

	if err := dFresh.ImportData(exportWithSecrets, RestoreModeReplace); err != nil {
		t.Fatalf("import into fresh db: %v", err)
	}

	freshConns, err := dFresh.ListConnections()
	if err != nil || len(freshConns) != 1 {
		t.Fatalf("expected 1 connection in fresh db, got %d (err: %v)", len(freshConns), err)
	}
	if freshConns[0].Data.APIKey != "sk-secret-key-12345" {
		t.Fatalf("expected secret APIKey restored, got %s", freshConns[0].Data.APIKey)
	}
	val, err := dFresh.KVGet("testScope", "hello")
	if err != nil || val != "world" {
		t.Fatalf("expected kv restored, got %s (err: %v)", val, err)
	}

	// 6. Test invalid schema version rejected
	exportWithSecrets.Version = 999
	if err := dFresh.ImportData(exportWithSecrets, RestoreModeReplace); err == nil {
		t.Fatalf("expected error for unsupported future version 999, got nil")
	}
}

func TestImportRollbackOnFailure(t *testing.T) {
	d := setupTestDB(t)

	// Seed one item
	if err := d.KVSet("keepMe", "key1", "value1"); err != nil {
		t.Fatalf("set kv: %v", err)
	}

	// Create payload with duplicate primary key error or invalid row
	badPayload := &ExportPayload{
		Version: CurrentExportVersion,
		Data: ExportData{
			KV: []KVEntry{
				{Scope: "newScope", Key: "k1", Value: "v1"},
			},
			Combos: []model.Combo{
				// Missing required name or valid data
				{ID: "c1", Name: "combo1", Models: []string{"m1"}},
				{ID: "c2", Name: "combo1", Models: []string{"m2"}}, // Duplicate name (UNIQUE constraint violation)
			},
		},
	}

	err := d.ImportData(badPayload, RestoreModeReplace)
	if err == nil {
		t.Fatalf("expected error on duplicate unique name combo, got nil")
	}

	// Ensure rollback preserved original state
	v, err := d.KVGet("keepMe", "key1")
	if err != nil || v != "value1" {
		t.Fatalf("expected transaction rollback to preserve keepMe key, got %s (err: %v)", v, err)
	}
}
