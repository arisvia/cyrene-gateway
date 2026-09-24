package db

import (
	"encoding/json"
	"strings"
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

func saveBackupUsage(t *testing.T, d *DB, e UsageEntry) {
	t.Helper()
	if err := d.SaveUsageEntry(&e); err != nil {
		t.Fatal(err)
	}
}

func exportBackupUsage(t *testing.T, d *DB) *ExportPayload {
	t.Helper()
	p, err := d.ExportData(true, true)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func assertBackupUsage(t *testing.T, d *DB, history, requests int, lifetime int64) {
	t.Helper()
	entries, err := d.GetUsageHistory(UsageFilter{Limit: 1000})
	if err != nil || len(entries) != history {
		t.Errorf("history = %d, want %d: %v", len(entries), history, err)
	}
	days, err := d.GetUsageDailyRange("")
	if err != nil {
		t.Fatal(err)
	}
	var n, prompt, completion int
	var cost float64
	for _, day := range days {
		n += day.Requests
		prompt += day.PromptTokens
		completion += day.CompletionTokens
		cost += day.Cost
		if day.ByProvider["p"].Requests != day.Requests || day.ByModel["m|p"].Requests != day.Requests {
			t.Errorf("daily dimensions diverged: %+v", day)
		}
	}
	if n != requests || prompt != requests*10 || completion != requests*5 || cost != float64(requests)*0.25 {
		t.Errorf("daily = %d/%d/%d/%g, want %d requests at 10/5/0.25", n, prompt, completion, cost, requests)
	}
	got, err := d.GetTotalRequestsLifetime()
	if err != nil || got != lifetime {
		t.Errorf("lifetime = %d, want %d: %v", got, lifetime, err)
	}
}

func TestUsageMergeExportThenLocalEvent(t *testing.T) {
	d := setupTestDB(t)
	e := UsageEntry{Timestamp: "2026-01-01T10:00:00Z", Provider: "p", Model: "m", APIKey: "a", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, d, e)
	backup := exportBackupUsage(t, d)
	e.APIKey = "b"
	saveBackupUsage(t, d, e)
	for range 2 {
		if err := d.ImportData(backup, RestoreModeMerge); err != nil {
			t.Fatal(err)
		}
		assertBackupUsage(t, d, 2, 2, 2)
	}
}

func TestUsageMergeMultisetAndForeignIDs(t *testing.T) {
	for _, field := range []string{"identical", "apiKey", "endpoint", "status", "meta", "tokens", "connection", "cost"} {
		t.Run(field, func(t *testing.T) {
			local, source := setupTestDB(t), setupTestDB(t)
			e := UsageEntry{Timestamp: "2026-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
			saveBackupUsage(t, local, e)
			switch field {
			case "apiKey":
				e.APIKey = "another-key"
			case "endpoint":
				e.Endpoint = "/v1/messages"
			case "status":
				e.Status = "error"
			case "meta":
				e.Meta = `{"request":"b"}`
			case "tokens":
				e.Tokens = `{"total_tokens":16}`
			case "connection":
				e.ConnectionID = "another-connection"
			case "cost":
				e.Cost = 0.5
			}
			saveBackupUsage(t, source, e)
			if field == "identical" {
				// Two legitimate identical rows must survive both imports.
				saveBackupUsage(t, source, e)
			}
			backup := exportBackupUsage(t, source)
			for range 2 {
				if err := local.ImportData(backup, RestoreModeMerge); err != nil {
					t.Fatal(err)
				}
				if field != "cost" {
					assertBackupUsage(t, local, 2, 2, 2)
				} else {
					day, err := local.GetUsageDaily("2026-01-01")
					if err != nil || day.Requests != 2 || day.Cost != 0.75 {
						t.Fatalf("cost identity lost: %+v: %v", day, err)
					}
				}
			}
		})
	}
}

func TestUsageMergePrunedAggregates(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		t.Run(map[bool]string{false: "disjoint", true: "ambiguous-overlap"}[overlap], func(t *testing.T) {
			local, source := setupTestDB(t), setupTestDB(t)
			e := UsageEntry{Timestamp: "2001-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
			saveBackupUsage(t, source, e)
			if _, _, err := source.PruneLogs(1); err != nil {
				t.Fatal(err)
			}
			if !overlap {
				e.Timestamp = "2001-01-02T10:00:00Z"
			}
			saveBackupUsage(t, local, e)
			if _, _, err := local.PruneLogs(1); err != nil {
				t.Fatal(err)
			}
			backup := exportBackupUsage(t, source)
			backup.Data.KV = []KVEntry{{Scope: "usage-test", Key: "rollback", Value: "must-not-leak"}}
			err := local.ImportData(backup, RestoreModeMerge)
			if overlap {
				if err == nil || !strings.Contains(err.Error(), "2001-01-01") || !strings.Contains(err.Error(), "replace") {
					t.Errorf("expected dated ambiguity error guiding replace, got %v", err)
				}
				if _, err := local.KVGet("usage-test", "rollback"); err != ErrNotFound {
					t.Errorf("KV not rolled back: %v", err)
				}
				assertBackupUsage(t, local, 0, 1, 1)
			} else {
				if err != nil {
					t.Fatal(err)
				}
				assertBackupUsage(t, local, 0, 2, 2)
				// A second aggregate-only import is ambiguous without provenance.
				if err := local.ImportData(backup, RestoreModeMerge); err == nil {
					t.Fatal("aggregate overlap silently accepted")
				}
				assertBackupUsage(t, local, 0, 2, 2)
			}
		})
	}
}

func TestUsageMergeLocalPrunedDayRejectsUnmatchedHistory(t *testing.T) {
	local, source := setupTestDB(t), setupTestDB(t)
	e := UsageEntry{Timestamp: "2001-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, local, e)
	saveBackupUsage(t, source, e)
	if _, _, err := local.PruneLogs(1); err != nil {
		t.Fatal(err)
	}
	if err := local.ImportData(exportBackupUsage(t, source), RestoreModeMerge); err == nil {
		t.Fatal("cannot tell whether imported history is already in pruned daily")
	}
	assertBackupUsage(t, local, 0, 1, 1)
}

func TestUsageReplacePreservesPrunedLifetime(t *testing.T) {
	local, source := setupTestDB(t), setupTestDB(t)
	e := UsageEntry{Timestamp: "2001-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, source, e)
	if _, _, err := source.PruneLogs(1); err != nil {
		t.Fatal(err)
	}
	e.Timestamp = "2026-01-02T10:00:00Z"
	saveBackupUsage(t, source, e)
	for range 3 {
		saveBackupUsage(t, local, e)
	}
	backup := exportBackupUsage(t, source)
	for range 2 {
		if err := local.ImportData(backup, RestoreModeReplace); err != nil {
			t.Fatal(err)
		}
		assertBackupUsage(t, local, 1, 2, 2)
	}
}

func TestUsageReplaceDoesNotApplyMergeOverlapValidation(t *testing.T) {
	local, source := setupTestDB(t), setupTestDB(t)
	e := UsageEntry{Timestamp: "2026-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, source, e)
	backup := exportBackupUsage(t, source)
	// Older snapshots can have undercounted daily data from the previous merge
	// bug. Replace restores the supplied rows verbatim, not a guessed repair.
	backup.Data.UsageHistory = append(backup.Data.UsageHistory, backup.Data.UsageHistory[0])
	if err := local.ImportData(backup, RestoreModeReplace); err != nil {
		t.Fatalf("replace rejected legacy snapshot: %v", err)
	}
	assertBackupUsage(t, local, 2, 1, 2)
}

func TestUsageMergePreservesExistingLifetimeBeyondDetails(t *testing.T) {
	local, source := setupTestDB(t), setupTestDB(t)
	e := UsageEntry{Timestamp: "2001-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, local, e)
	if _, _, err := local.PruneLogs(1); err != nil {
		t.Fatal(err)
	}
	if _, err := local.conn.Exec(`UPDATE _meta SET value = '9' WHERE key = 'totalRequestsLifetime'`); err != nil {
		t.Fatal(err)
	}
	e.Timestamp = "2001-01-02T10:00:00Z"
	saveBackupUsage(t, source, e)
	if err := local.ImportData(exportBackupUsage(t, source), RestoreModeMerge); err != nil {
		t.Fatal(err)
	}
	assertBackupUsage(t, local, 1, 2, 10)
}

func TestUsageMergeWithoutDailyRebuildsNewEvents(t *testing.T) {
	local, source := setupTestDB(t), setupTestDB(t)
	e := UsageEntry{Timestamp: "2026-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, source, e)
	backup := exportBackupUsage(t, source)
	backup.Data.UsageDaily = nil
	for range 2 {
		if err := local.ImportData(backup, RestoreModeMerge); err != nil {
			t.Fatal(err)
		}
		assertBackupUsage(t, local, 1, 1, 1)
	}
}

func TestUsageMergeInvalidDailyRollsBack(t *testing.T) {
	for _, raw := range []string{`not-json`, `{"requests":0}`, `{"requests":1,"promptTokens":10,"completionTokens":5,"cost":0.25,"byProvider":{"wrong":{"requests":1}}}`} {
		t.Run(raw, func(t *testing.T) {
			local, source := setupTestDB(t), setupTestDB(t)
			e := UsageEntry{Timestamp: "2026-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
			saveBackupUsage(t, source, e)
			backup := exportBackupUsage(t, source)
			backup.Data.UsageDaily[0].Data = raw
			if err := local.ImportData(backup, RestoreModeMerge); err == nil {
				t.Fatal("invalid daily accepted")
			}
			assertBackupUsage(t, local, 0, 0, 0)
		})
	}
}

func TestUsageMergeCompleteSubsetOfPartiallyPrunedDay(t *testing.T) {
	local := setupTestDB(t)
	e := UsageEntry{Timestamp: "2026-01-01T10:00:00Z", Provider: "p", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.25}
	saveBackupUsage(t, local, e)
	backup := exportBackupUsage(t, local)
	e.APIKey = "later-pruned"
	saveBackupUsage(t, local, e)
	if _, err := local.conn.Exec(`DELETE FROM usageHistory WHERE apiKey = 'later-pruned'`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := local.ImportData(backup, RestoreModeMerge); err != nil {
			t.Fatal(err)
		}
		assertBackupUsage(t, local, 1, 2, 2)
	}
	// Equal request totals alone cannot prove dimension completeness.
	var day DayData
	if err := json.Unmarshal([]byte(backup.Data.UsageDaily[0].Data), &day); err != nil {
		t.Fatal(err)
	}
	day.ByProvider["p"] = DayCounter{Requests: 2, PromptTokens: 20, CompletionTokens: 10, Cost: 0.5}
	data, err := json.Marshal(day)
	if err != nil {
		t.Fatal(err)
	}
	backup.Data.UsageDaily[0].Data = string(data)
	if err := local.ImportData(backup, RestoreModeMerge); err == nil {
		t.Fatal("inconsistent dimensions silently accepted")
	}
}
