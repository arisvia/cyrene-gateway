package db

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func TestAPIKey_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	key := &model.APIKey{
		ID:            "key-1",
		Key:           "cg-testkey123.sig",
		Name:          "Test Key",
		MachineID:     "mac-1",
		IsActive:      true,
		AllowedModels: []string{"deepseek/*", "openai/gpt-4o"},
		RPM:           60,
		SystemPrompt:  "You are a helpful assistant.",
		ExpiresAt:     time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}

	// 1. Create
	if err := database.CreateAPIKey(key); err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// 2. Validate
	valid, err := database.ValidateAPIKey(key.Key)
	if err != nil || !valid {
		t.Fatalf("ValidateAPIKey failed: valid=%v, err=%v", valid, err)
	}

	// 3. Get by Key
	gotByKey, err := database.GetAPIKeyByKey(key.Key)
	if err != nil || gotByKey == nil {
		t.Fatalf("GetAPIKeyByKey failed: %v", err)
	}
	if gotByKey.ID != key.ID || gotByKey.Name != key.Name || gotByKey.RPM != 60 || gotByKey.SystemPrompt != key.SystemPrompt {
		t.Fatalf("GetAPIKeyByKey mismatch: %+v", gotByKey)
	}
	if !reflect.DeepEqual(gotByKey.AllowedModels, key.AllowedModels) {
		t.Fatalf("AllowedModels mismatch: got %v, want %v", gotByKey.AllowedModels, key.AllowedModels)
	}

	// 4. Get by ID
	gotByID, err := database.GetAPIKey(key.ID)
	if err != nil || gotByID == nil {
		t.Fatalf("GetAPIKey failed: %v", err)
	}
	if gotByID.Key != key.Key {
		t.Fatalf("GetAPIKey key mismatch: %s != %s", gotByID.Key, key.Key)
	}

	// 5. Update
	key.Name = "Updated Key"
	key.RPM = 120
	key.AllowedModels = []string{"*"}
	if err := database.UpdateAPIKey(key); err != nil {
		t.Fatalf("UpdateAPIKey failed: %v", err)
	}

	gotUpdated, err := database.GetAPIKey(key.ID)
	if err != nil || gotUpdated == nil {
		t.Fatalf("GetAPIKey after update failed: %v", err)
	}
	if gotUpdated.Name != "Updated Key" || gotUpdated.RPM != 120 || len(gotUpdated.AllowedModels) != 1 || gotUpdated.AllowedModels[0] != "*" {
		t.Fatalf("UpdateAPIKey fields not persisted: %+v", gotUpdated)
	}

	// 6. List
	keys, err := database.ListAPIKeys()
	if err != nil {
		t.Fatalf("ListAPIKeys failed: %v", err)
	}
	if len(keys) != 1 || keys[0].ID != key.ID {
		t.Fatalf("ListAPIKeys mismatch: %v", keys)
	}

	// 7. Delete
	if err := database.DeleteAPIKey(key.ID); err != nil {
		t.Fatalf("DeleteAPIKey failed: %v", err)
	}

	gotDeleted, err := database.GetAPIKey(key.ID)
	if err != nil {
		t.Fatalf("GetAPIKey after delete unexpected error: %v", err)
	}
	if gotDeleted != nil {
		t.Fatalf("expected nil after delete, got %+v", gotDeleted)
	}
}
