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
