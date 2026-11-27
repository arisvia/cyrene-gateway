package db

import "testing"

// Stored rows predate loopGuardEnabled, so the default must come from
// DefaultSettings() rather than the JSON zero value.
func TestLoopGuardEnabledDefaultsTrueForLegacyRow(t *testing.T) {
	d := setupTestDB(t)
	if _, err := d.conn.Exec(
		`INSERT INTO settings (id, data) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		`{"comboStrategy":"fallback","rtkEnabled":true,"responseCacheTTL":60}`,
	); err != nil {
		t.Fatal(err)
	}

	settings, err := d.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.LoopGuardEnabled {
		t.Error("loop guard must default to enabled for a stored row without the key")
	}
	if !settings.RTKEnabled || settings.ResponseCacheTTL != 60 {
		t.Errorf("stored fields were overwritten by defaults: %+v", settings)
	}

	settings.LoopGuardEnabled = false
	if err := d.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	reloaded, err := d.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.LoopGuardEnabled {
		t.Error("disabling the loop guard must persist as false")
	}
	if !reloaded.RTKEnabled || reloaded.ResponseCacheTTL != 60 {
		t.Errorf("other settings changed while persisting: %+v", reloaded)
	}
}

func TestDefaultSettingsLoopGuardEnabled(t *testing.T) {
	if !DefaultSettings().LoopGuardEnabled {
		t.Error("a fresh install must have the loop guard enabled")
	}
}

func TestSettingsAndKVCacheIsolationAndInvalidation(t *testing.T) {
	d := setupTestDB(t)

	// Warm settings cache and verify returned clone isolation
	s1, err := d.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	s1.TokenSaverExclude = append(s1.TokenSaverExclude, "mutated")
	s1.RTKEnabled = true

	s2, err := d.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if s2.RTKEnabled || len(s2.TokenSaverExclude) != 0 {
		t.Fatalf("cache was mutated through caller pointer: %+v", s2)
	}

	s2.RTKEnabled = true
	if err := d.SaveSettings(s2); err != nil {
		t.Fatal(err)
	}
	s3, err := d.GetSettings()
	if err != nil || !s3.RTKEnabled {
		t.Fatalf("SaveSettings did not update cache: %+v err=%v", s3, err)
	}

	// KV cache population, clone isolation, and invalidation on every write verb
	if err := d.KVSet("disabled", "m1", "1"); err != nil {
		t.Fatal(err)
	}
	list1, err := d.KVList("disabled")
	if err != nil || list1["m1"] != "1" {
		t.Fatalf("KVList initial failed: %+v err=%v", list1, err)
	}
	list1["m1"] = "poison"
	if v, err := d.KVGet("disabled", "m1"); err != nil || v != "1" {
		t.Fatalf("KVGet affected by caller map mutation: v=%q err=%v", v, err)
	}

	if err := d.KVBatchSet("disabled", []string{"m2", "m3"}, "1"); err != nil {
		t.Fatal(err)
	}
	list2, err := d.KVList("disabled")
	if err != nil || len(list2) != 3 {
		t.Fatalf("KVBatchSet did not invalidate KVList cache: %+v err=%v", list2, err)
	}

	if err := d.KVDelete("disabled", "m1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.KVGet("disabled", "m1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after KVDelete, got %v", err)
	}

	if err := d.KVBatchDelete("disabled", []string{"m2", "m3"}); err != nil {
		t.Fatal(err)
	}
	list3, err := d.KVList("disabled")
	if err != nil || len(list3) != 0 {
		t.Fatalf("KVBatchDelete did not invalidate cache: %+v err=%v", list3, err)
	}
}
