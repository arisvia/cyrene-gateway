package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gatewaydb "github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

func newMigrationDB(t *testing.T) (*gatewaydb.DB, string) {
	t.Helper()
	dir := t.TempDir()
	database, err := gatewaydb.Open(filepath.Join(dir, "data.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database, dir
}

func execSQL(t *testing.T, database *gatewaydb.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Conn().Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func queryString(t *testing.T, database *gatewaydb.DB, query string, args ...any) string {
	t.Helper()
	var value string
	if err := database.Conn().QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

const (
	intlBaseURL  = `{"baseUrl":"https://coding-intl.dashscope.aliyuncs.com/v1/chat/completions","apiKey":"fixture-only"}`
	chinaBaseURL = `{"baseUrl":"https://coding.dashscope.aliyuncs.com/v1/chat/completions","apiKey":"fixture-only"}`
)

func seedConnectionData(t *testing.T, database *gatewaydb.DB, id, providerID, auth, data string) {
	t.Helper()
	execSQL(t, database, `INSERT INTO providerConnections
		(id, provider, authType, isActive, data, createdAt, updatedAt)
		VALUES (?, ?, ?, 1, ?, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')`, id, providerID, auth, data)
}

func seedConnection(t *testing.T, database *gatewaydb.DB, id, providerID, auth string) {
	t.Helper()
	seedConnectionData(t, database, id, providerID, auth, `{"apiKey":"fixture-only","extra":9007199254740993}`)
}

// Policy surfaces alone are not pre-canonical evidence, so single-purpose tests
// need one legacy row to reach the renames at all.
func seedPreCanonical(t *testing.T, database *gatewaydb.DB) {
	t.Helper()
	seedConnection(t, database, "pre-canonical", "codebuddy-intl", "oauth")
}

func seedRequestDetail(t *testing.T, database *gatewaydb.DB, id, providerID, model, connectionID string) {
	t.Helper()
	raw := fmt.Sprintf(`{"provider":%q,"model":%q}`, providerID, model)
	execSQL(t, database, `INSERT INTO requestDetails (id, timestamp, provider, model, connectionId, status, data)
		VALUES (?, '2026-09-01T00:00:00Z', ?, ?, ?, 'ok', ?)`, id, providerID, model, connectionID, raw)
}

func seedUsage(t *testing.T, database *gatewaydb.DB, providerID, model, connectionID string) {
	t.Helper()
	if err := database.SaveUsageEntry(&gatewaydb.UsageEntry{Timestamp: "2026-09-01T00:00:00Z", Provider: providerID, Model: model, ConnectionID: connectionID}); err != nil {
		t.Fatal(err)
	}
}

func migrationCLI(dir string) ([]byte, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(executable, "-test.run=^TestMigrationCLIProcess$")
	cmd.Env = append(os.Environ(), "CYRENE_MIGRATION_TEST_DIR="+dir)
	return cmd.CombinedOutput()
}

func TestMigrationCLIProcess(t *testing.T) {
	if dir := os.Getenv("CYRENE_MIGRATION_TEST_DIR"); dir != "" {
		flag.CommandLine = flag.NewFlagSet("migrate", flag.ExitOnError)
		os.Args = []string{"migrate", "-data-dir", dir}
		main()
	}
}

func TestMigrateLegacyRegionsAndIdempotence(t *testing.T) {
	database, _ := newMigrationDB(t)
	cases := []struct{ id, old, want string }{
		{"china", "alicode", "alicode-cn"},
		{"intl", "alicode-intl", "alicode"},
		{"buddy", "codebuddy-intl", "codebuddy"},
		{"other", "codebuddy-cn", "codebuddy-cn"},
	}
	for _, tc := range cases {
		seedConnection(t, database, tc.id, tc.old, "api-key")
		raw := fmt.Sprintf(`{"provider":%q,"model":%q,"input":"alicode-intl/do-not-rewrite","extra":9007199254740993}`, tc.old, tc.old+"/vendor/model[1m](high)")
		execSQL(t, database, `INSERT INTO requestDetails (id, timestamp, provider, model, connectionId, status, data) VALUES (?, '2026-09-01T00:00:00Z', ?, ?, ?, 'ok', ?)`, tc.id, tc.old, tc.old+"/vendor/model[1m](high)", tc.id, raw)
		if err := database.SaveUsageEntry(&gatewaydb.UsageEntry{Timestamp: "2026-09-01T00:00:00Z", Provider: tc.old, Model: tc.old + "/vendor/model[1m](high)", ConnectionID: tc.id}); err != nil {
			t.Fatal(err)
		}
	}
	seedConnection(t, database, "no-auth", "opencode", "none")
	seedConnection(t, database, "has-key", "opencode", "api-key")
	seedConnection(t, database, "other-none", "openai", "none")
	// A same-named key in another scope must not act as the migration marker.
	execSQL(t, database, `INSERT INTO kv VALUES ('other', 'provider-ids-v1', '1')`)

	for attempt := 0; attempt < 2; attempt++ {
		if err := migrate(database.Conn()); err != nil {
			t.Fatal(err)
		}
		for _, tc := range cases {
			for _, table := range []string{"providerConnections", "requestDetails", "usageHistory"} {
				column := "id"
				if table == "usageHistory" {
					column = "connectionId"
				}
				got := queryString(t, database, `SELECT provider FROM `+table+` WHERE `+column+` = ?`, tc.id)
				if got != tc.want {
					t.Errorf("attempt %d %s/%s provider = %q, want %q", attempt, table, tc.id, got, tc.want)
				}
			}
			for _, table := range []string{"requestDetails", "usageHistory"} {
				got := queryString(t, database, `SELECT model FROM `+table+` WHERE connectionId = ?`, tc.id)
				if got != tc.want+"/vendor/model[1m](high)" {
					t.Errorf("%s model = %q", table, got)
				}
			}
			assertJSON(t, queryString(t, database, `SELECT data FROM requestDetails WHERE id = ?`, tc.id), fmt.Sprintf(`{"provider":%q,"model":%q,"input":"alicode-intl/do-not-rewrite","extra":9007199254740993}`, tc.want, tc.want+"/vendor/model[1m](high)"))
			if got := queryString(t, database, `SELECT data FROM providerConnections WHERE id = ?`, tc.id); got != `{"apiKey":"fixture-only","extra":9007199254740993}` {
				t.Errorf("credentials changed: %s", got)
			}
		}
	}
	if got := queryString(t, database, `SELECT authType || ':' || isActive FROM providerConnections WHERE id = 'no-auth'`); got != "api-key:0" {
		t.Errorf("OpenCode none auth = %s", got)
	}
	for id, want := range map[string]string{"has-key": "api-key:1", "other-none": "none:1"} {
		if got := queryString(t, database, `SELECT authType || ':' || isActive FROM providerConnections WHERE id = ?`, id); got != want {
			t.Errorf("%s auth = %s", id, got)
		}
	}
	assertMarker(t, database, true)
}

func TestMigrateAliasesAndCombosResolveWithoutCache(t *testing.T) {
	database, _ := newMigrationDB(t)
	for key, target := range map[string]string{
		"china": "alicode/vendor/m[1m](high)", "intl": "alicode-intl/vendor/m",
		"buddy": "codebuddy-intl/vendor/m", "bare": "alicode-intl",
		"vendor": "openrouter/alicode-intl/m", "similar": "alicode-intl-extra/m",
	} {
		execSQL(t, database, `INSERT INTO kv VALUES ('aliases', ?, ?)`, key, target)
	}
	execSQL(t, database, `INSERT INTO kv VALUES ('unrelated', 'alicode-intl', 'alicode-intl/m')`)
	execSQL(t, database, `INSERT INTO combos VALUES ('c', 'fallback', '', '["alicode/vendor/m[1m](high)","alicode-intl/vendor/m","codebuddy-intl/vendor/m","china","openrouter/alicode-intl/m","alicode-intl/vendor/m"]', '', '')`)
	if err := migrate(database.Conn()); err != nil {
		t.Fatal(err)
	}
	for alias, want := range map[string]string{"china": "alicode-cn", "intl": "alicode", "buddy": "codebuddy"} {
		got, err := provider.ResolveModel(alias, database)
		if err != nil || got.Provider != want || got.Model != "vendor/m" {
			t.Errorf("resolve %s = %+v, %v; want %s/vendor/m", alias, got, err, want)
		}
	}
	combo, ok := provider.ResolveCombo("fallback", database)
	wantModels := []string{"alicode-cn/vendor/m[1m](high)", "alicode/vendor/m", "codebuddy/vendor/m", "china", "openrouter/alicode-intl/m", "alicode/vendor/m"}
	if !ok || !reflect.DeepEqual(combo.Models, wantModels) {
		t.Fatalf("combo = %+v, want %v", combo, wantModels)
	}
	for i, want := range []string{"alicode-cn", "alicode", "codebuddy", "alicode-cn", "openrouter", "alicode"} {
		got, err := provider.ResolveModel(combo.Models[i], database)
		if err != nil || got.Provider != want {
			t.Errorf("resolve combo[%d] = %+v, %v", i, got, err)
		}
	}
	for key, want := range map[string]string{"bare": "alicode-intl", "vendor": "openrouter/alicode-intl/m", "similar": "alicode-intl-extra/m"} {
		if got := queryString(t, database, `SELECT value FROM kv WHERE scope = 'aliases' AND key = ?`, key); got != want {
			t.Errorf("unrelated alias %s changed to %s", key, got)
		}
	}
	if got := queryString(t, database, `SELECT value FROM kv WHERE scope = 'unrelated'`); got != "alicode-intl/m" {
		t.Error("unrelated KV scope changed")
	}
	if got := queryString(t, database, `SELECT count(*) FROM kv WHERE scope = 'providerModelCache'`); got != "0" {
		t.Fatal("resolution fixture must have an empty cache")
	}
}

func TestMigrateCacheCollisionsPreserveCanonicalDestination(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(fmt.Sprint(collision), func(t *testing.T) {
			database, _ := newMigrationDB(t)
			for key, value := range map[string]string{"alicode": "china", "alicode-intl": "intl", "codebuddy-intl": "buddy-legacy", "unrelated": "other"} {
				execSQL(t, database, `INSERT INTO kv VALUES ('providerModelCache', ?, ?)`, key, `{"models":[{"id":"`+value+`"}]}`)
			}
			if collision {
				execSQL(t, database, `INSERT INTO kv VALUES ('providerModelCache', 'alicode-cn', '{"models":[{"id":"china-canonical"}]}'), ('providerModelCache', 'codebuddy', '{"models":[{"id":"buddy-canonical"}]}')`)
			}
			if err := migrate(database.Conn()); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"alicode-cn": "china", "alicode": "intl", "codebuddy": "buddy-legacy", "unrelated": "other"}
			if collision {
				want["alicode-cn"], want["codebuddy"] = "china-canonical", "buddy-canonical"
			}
			for key, value := range want {
				if got := queryString(t, database, `SELECT value FROM kv WHERE scope = 'providerModelCache' AND key = ?`, key); got != `{"models":[{"id":"`+value+`"}]}` {
					t.Errorf("cache %s = %s", key, got)
				}
			}
			if got := queryString(t, database, `SELECT count(*) FROM kv WHERE scope = 'providerModelCache'`); got != "4" {
				t.Errorf("cache count = %s, want 4", got)
			}
		})
	}
}

func TestMigrateModelKeysPreserveRestrictionsAndOpaqueMetadata(t *testing.T) {
	database, _ := newMigrationDB(t)
	for _, key := range []string{"alicode/cn-only", "alicode-intl/intl-only", "codebuddy-intl/buddy-only", "alicode/collision", "alicode-cn/collision", "codebuddy-intl/collision", "codebuddy/collision", "bare", "alicode-intl", "alicode-intl-extra/m"} {
		execSQL(t, database, `INSERT INTO kv VALUES ('disabledModels', ?, 'true')`, key)
	}
	execSQL(t, database, `INSERT INTO kv VALUES ('disabledModels', 'alicode-cn/cn-only', '')`)
	metadata := map[string]string{
		"alicode/m":        ` {"displayName":"alicode-intl/opaque","contextLength":32000,"maxOutputTokens":4096,"extra":9007199254740993} `,
		"alicode-intl/m":   `{"displayName":"Intl"}`,
		"codebuddy-intl/m": `{"displayName":"Legacy"}`,
		"codebuddy/m":      ` {"displayName":"Canonical","future":{"secret":"fixture-only"}} `,
		"bare":             "opaque", "alicode-intl-extra/m": "untouched",
	}
	for key, value := range metadata {
		execSQL(t, database, `INSERT INTO kv VALUES ('modelMetaOverrides', ?, ?)`, key, value)
	}
	execSQL(t, database, `INSERT INTO kv VALUES ('unrelated', 'alicode-intl/m', 'opaque')`)
	for attempt := 0; attempt < 2; attempt++ {
		if err := migrate(database.Conn()); err != nil {
			t.Fatal(err)
		}
		for fullID, want := range map[string]bool{
			"alicode-cn/cn-only": true, "alicode/cn-only": false,
			"alicode/intl-only": true, "alicode-cn/intl-only": false,
			"codebuddy/buddy-only": true, "codebuddy-cn/buddy-only": false,
			"alicode-cn/collision": true, "alicode/collision": false, "codebuddy/collision": true,
			"openai/bare": true, "bare": true, "alicode-intl": true, "alicode-intl-extra/m": true,
		} {
			if got := provider.IsModelDisabled(fullID, database); got != want {
				t.Errorf("attempt %d disabled %s = %v, want %v", attempt, fullID, got, want)
			}
		}
		disabled, err := database.KVList("disabledModels")
		if err != nil {
			t.Fatal(err)
		}
		wantDisabled := map[string]string{"alicode-cn/cn-only": "true", "alicode/intl-only": "true", "codebuddy/buddy-only": "true", "alicode-cn/collision": "true", "codebuddy/collision": "true", "bare": "true", "alicode-intl": "true", "alicode-intl-extra/m": "true"}
		if !reflect.DeepEqual(disabled, wantDisabled) {
			t.Errorf("disabled keys = %v, want %v", disabled, wantDisabled)
		}
		got, err := database.KVList("modelMetaOverrides")
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"alicode-cn/m": metadata["alicode/m"], "alicode/m": metadata["alicode-intl/m"], "codebuddy/m": metadata["codebuddy/m"], "bare": "opaque", "alicode-intl-extra/m": "untouched"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("metadata = %v, want %v", got, want)
		}
		if got := queryString(t, database, `SELECT value FROM kv WHERE scope = 'unrelated' AND key = 'alicode-intl/m'`); got != "opaque" {
			t.Errorf("unrelated scope changed: %s", got)
		}
	}
}

func TestMigrateAPIAllowlistPreservesRegionalAccess(t *testing.T) {
	references := []struct{ old, current string }{
		{"alicode/vendor/m", "alicode-cn/vendor/m"}, {"alicode/vendor/more", "alicode-cn/vendor/more"},
		{"alicode-intl/vendor/m", "alicode/vendor/m"}, {"alicode-intl/vendor/more", "alicode/vendor/more"},
		{"codebuddy-intl/vendor/m", "codebuddy/vendor/m"}, {"codebuddy-cn/vendor/m", "codebuddy-cn/vendor/m"},
		{"openai/vendor/m", "openai/vendor/m"}, {"alicode-intl-extra/vendor/m", "alicode-intl-extra/vendor/m"},
		{"codebuddy-intl-extra/vendor/m", "codebuddy-intl-extra/vendor/m"}, {"alicode-cn-extra/vendor/m", "alicode-cn-extra/vendor/m"},
		{"vendor/alicode-intl/m", "vendor/alicode-intl/m"}, {"alicode", "alicode"}, {"alicode-intl", "alicode-intl"},
		{"codebuddy-intl", "codebuddy-intl"}, {"foo", "foo"}, {"", ""},
	}
	// narrows records the IDs a provider-scoped wildcard intentionally stops
	// matching: legacy strings that were never routable provider prefixes.
	for _, tc := range []struct {
		name    string
		raw     string
		narrows map[string]bool
	}{
		{"cn-exact", `[" alicode/vendor/m "]`, nil}, {"intl-exact-folded", `[" ALICODE-INTL/VENDOR/M "]`, nil},
		{"buddy-exact-folded", `[" CodeBuddy-Intl/vendor/m "]`, nil}, {"cn-models", `[" alicode/* "]`, nil},
		{"intl-models", `["alicode-intl/*"]`, nil}, {"buddy-models", `["codebuddy-intl/*"]`, nil},
		{"ali-family", `[" alicode* "]`, map[string]bool{"alicode-intl-extra/vendor/m": false, "alicode-cn-extra/vendor/m": false, "alicode": false, "alicode-intl": false}},
		{"intl-family", `["alicode-intl*"]`, map[string]bool{"alicode-intl-extra/vendor/m": false, "alicode-intl": false}},
		{"buddy-family", `["codebuddy-intl*"]`, map[string]bool{"codebuddy-intl-extra/vendor/m": false, "codebuddy-intl": false}},
		{"buddy-all-regions", `["codebuddy*"]`, map[string]bool{"codebuddy-intl-extra/vendor/m": false, "codebuddy-intl": false}},
		{"model-prefix", `["alicode-intl/vendor/m*"]`, nil},
		{"case-sensitive-wildcards", `["ALICODE-INTL/*","CODEBUDDY-INTL*"]`, nil},
		{"unknown-and-bare", `["alicode-intl-extra/*","vendor/alicode-intl*","alicode","foo"]`, nil},
		{"mixed", `["alicode/vendor/m","alicode-intl/vendor/m","codebuddy-intl/*","alicode/vendor/m"]`, nil},
		{"all", `["*"]`, nil}, {"blank-rule", `[" "]`, nil}, {"empty-array", ` [ ] `, nil}, {"json-null", `null`, nil}, {"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, _ := newMigrationDB(t)
			seedPreCanonical(t, database)
			execSQL(t, database, `INSERT INTO apiKeys (id, key, allowedModels, rpm, systemPrompt, expiresAt, createdAt) VALUES ('k', 'fixture-only', ?, 17, 'alicode-intl/opaque', '2030-01-01', '2026-09-01')`, tc.raw)
			var original model.APIKey
			if tc.raw != "" {
				if err := json.Unmarshal([]byte(tc.raw), &original.AllowedModels); err != nil {
					t.Fatal(err)
				}
			}
			var first string
			for attempt := 0; attempt < 2; attempt++ {
				if err := migrate(database.Conn()); err != nil {
					t.Fatal(err)
				}
				raw := queryString(t, database, `SELECT allowedModels FROM apiKeys WHERE id = 'k'`)
				var current model.APIKey
				if raw != "" {
					if err := json.Unmarshal([]byte(raw), &current.AllowedModels); err != nil {
						t.Fatal(err)
					}
				}
				for _, ref := range references {
					want, narrowed := tc.narrows[ref.old]
					if !narrowed {
						want = original.IsModelAllowed(ref.old)
					} else if original.IsModelAllowed(ref.old) == want {
						t.Fatalf("stale narrowing expectation for %s", ref.old)
					}
					if got := current.IsModelAllowed(ref.current); got != want {
						t.Errorf("attempt %d rules %s allow %s = %v, want %v (originally %s = %v)", attempt, raw, ref.current, got, want, ref.old, original.IsModelAllowed(ref.old))
					}
				}
				if attempt == 0 {
					first = raw
				} else if raw != first {
					t.Error("rerun changed allowlist")
				}
			}
			if got := queryString(t, database, `SELECT key || ':' || rpm || ':' || systemPrompt || ':' || expiresAt || ':' || createdAt FROM apiKeys WHERE id = 'k'`); got != "fixture-only:17:alicode-intl/opaque:2030-01-01:2026-09-01" {
				t.Errorf("API key fields changed: %s", got)
			}
		})
	}
	t.Run("sql-null", func(t *testing.T) {
		database, _ := newMigrationDB(t)
		seedPreCanonical(t, database)
		execSQL(t, database, `INSERT INTO apiKeys (id, key, allowedModels, createdAt) VALUES ('k', 'fixture-only', NULL, '')`)
		if err := migrate(database.Conn()); err != nil {
			t.Fatal(err)
		}
		if got := queryString(t, database, `SELECT allowedModels IS NULL FROM apiKeys WHERE id = 'k'`); got != "1" {
			t.Error("SQL NULL changed")
		}
	})
}

func TestMigrateAPIAllowlistWildcardRegionPolicy(t *testing.T) {
	type probe struct {
		legacy, current string
		want            bool
	}
	for _, tc := range []struct {
		name   string
		raw    string
		probes []probe
	}{
		{"alicode-dash-wildcard", `["alicode-*"]`, []probe{
			{"alicode-intl/vendor/m", "alicode/vendor/m", true},
			{"alicode-intl/vendor/more", "alicode/vendor/more", true},
			{"alicode/vendor/m", "alicode-cn/vendor/m", false},
			{"codebuddy-intl/vendor/m", "codebuddy/vendor/m", false},
		}},
		{"codebuddy-i-wildcard", `["codebuddy-i*"]`, []probe{
			{"codebuddy-intl/vendor/m", "codebuddy/vendor/m", true},
			{"codebuddy-cn/vendor/m", "codebuddy-cn/vendor/m", false},
			{"alicode-intl/vendor/m", "alicode/vendor/m", false},
		}},
		{"alicode-family-wildcard", `[" alicode* "]`, []probe{
			{"alicode/vendor/m", "alicode-cn/vendor/m", true},
			{"alicode-intl/vendor/m", "alicode/vendor/m", true},
			{"codebuddy-intl/vendor/m", "codebuddy/vendor/m", false},
			{"openai/vendor/m", "openai/vendor/m", false},
		}},
		{"codebuddy-family-wildcard", `["codebuddy*"]`, []probe{
			{"codebuddy-intl/vendor/m", "codebuddy/vendor/m", true},
			{"codebuddy-cn/vendor/m", "codebuddy-cn/vendor/m", true},
			{"alicode-intl/vendor/m", "alicode/vendor/m", false},
		}},
		{"alicode-intl-family-wildcard", `["alicode-intl*"]`, []probe{
			{"alicode-intl/vendor/m", "alicode/vendor/m", true},
			{"alicode/vendor/m", "alicode-cn/vendor/m", false},
		}},
		{"allow-all", `["*"]`, []probe{
			{"alicode-intl/vendor/m", "alicode/vendor/m", true},
			{"alicode/vendor/m", "alicode-cn/vendor/m", true},
			{"codebuddy-cn/vendor/m", "codebuddy-cn/vendor/m", true},
		}},
		{"unrelated-providers", `["qoder/*","deepseek/glm-5","kimi*"]`, []probe{
			{"qoder/vendor/m", "qoder/vendor/m", true},
			{"kimi/vendor/m", "kimi/vendor/m", true},
			{"deepseek/glm-5", "deepseek/glm-5", true},
			{"alicode-intl/vendor/m", "alicode/vendor/m", false},
			{"alicode/vendor/m", "alicode-cn/vendor/m", false},
		}},
		{"intl-slash-wildcard", `["alicode-intl/*"]`, []probe{
			{"alicode-intl/vendor/m", "alicode/vendor/m", true},
			{"alicode/vendor/m", "alicode-cn/vendor/m", false},
		}},
		{"legacy-exact-model", `["codebuddy-intl/glm-5"]`, []probe{
			{"codebuddy-intl/glm-5", "codebuddy/glm-5", true},
			{"codebuddy-intl/glm-4", "codebuddy/glm-4", false},
			{"codebuddy-cn/glm-5", "codebuddy-cn/glm-5", false},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, _ := newMigrationDB(t)
			seedPreCanonical(t, database)
			execSQL(t, database, `INSERT INTO apiKeys (id, key, allowedModels, createdAt) VALUES ('k', 'fixture-only', ?, '')`, tc.raw)
			var original model.APIKey
			if err := json.Unmarshal([]byte(tc.raw), &original.AllowedModels); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := migrate(database.Conn()); err != nil {
					t.Fatal(err)
				}
				var current model.APIKey
				if err := json.Unmarshal([]byte(queryString(t, database, `SELECT allowedModels FROM apiKeys WHERE id = 'k'`)), &current.AllowedModels); err != nil {
					t.Fatal(err)
				}
				for _, p := range tc.probes {
					if attempt == 0 {
						if got := original.IsModelAllowed(p.legacy); got != p.want {
							t.Errorf("legacy rule %s allow %s = %v, want %v", tc.raw, p.legacy, got, p.want)
						}
					}
					if got := current.IsModelAllowed(p.current); got != p.want {
						t.Errorf("attempt %d migrated rules %v allow %s = %v, want %v", attempt, current.AllowedModels, p.current, got, p.want)
					}
				}
			}
		})
	}
}

func TestMigrateTokenSaverExclusionsPreservesOtherSettings(t *testing.T) {
	database, _ := newMigrationDB(t)
	seedPreCanonical(t, database)
	execSQL(t, database, `INSERT INTO settings VALUES (1, '{"tokenSaverExclude":[" ALICODE ","alicode-intl","CodeBuddy-Intl/vendor/M"," ALICODE-INTL/vendor/M ","bare","alicode-intl-extra/m","openrouter/alicode-intl/m"],"passwordHash":"fixture-only","requireLogin":true,"future":{"n":9007199254740993,"text":"alicode-intl"},"providerStrategies":{"alicode":{"future":"opaque"}},"providerThinking":null}')`)
	var first string
	for attempt := 0; attempt < 2; attempt++ {
		if err := migrate(database.Conn()); err != nil {
			t.Fatal(err)
		}
		raw := queryString(t, database, `SELECT data FROM settings WHERE id = 1`)
		assertJSON(t, raw, `{"tokenSaverExclude":[" alicode-cn ","alicode","codebuddy/vendor/M"," alicode/vendor/M ","bare","alicode-intl-extra/m","openrouter/alicode-intl/m"],"passwordHash":"fixture-only","requireLogin":true,"future":{"n":9007199254740993,"text":"alicode-intl"},"providerStrategies":{"alicode":{"future":"opaque"}},"providerThinking":null}`)
		if attempt == 0 {
			first = raw
		} else if raw != first {
			t.Error("rerun changed settings")
		}
	}
}

func TestMigrateMalformedPolicyJSONRollsBack(t *testing.T) {
	for _, tc := range []struct{ name, query, raw string }{
		{"allowlist", `INSERT INTO apiKeys (id, key, allowedModels, createdAt) VALUES ('k', 'fixture-only', ?, '')`, `["alicode/m",42]`},
		{"exclusions", `INSERT INTO settings VALUES (1, ?)`, `{"tokenSaverExclude":{},"passwordHash":"fixture-only"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, _ := newMigrationDB(t)
			seedPreCanonical(t, database)
			seedConnection(t, database, "cn", "alicode", "api-key")
			execSQL(t, database, tc.query, tc.raw)
			if err := migrate(database.Conn()); err == nil {
				t.Error("migration accepted malformed policy JSON")
			}
			if got := queryString(t, database, `SELECT provider FROM providerConnections WHERE id = 'cn'`); got != "alicode" {
				t.Errorf("failed migration changed provider to %s", got)
			}
			assertMarker(t, database, false)
		})
	}
}

func TestMigrateDailyDimensionsMergeAndBackupReconciles(t *testing.T) {
	database, _ := newMigrationDB(t)
	for _, item := range []struct{ provider, model string }{
		{"alicode", "alicode/vendor/m"}, {"alicode-cn", "alicode-cn/vendor/m"},
		{"alicode-intl", "alicode-intl/vendor/m"}, {"codebuddy-intl", "m"}, {"codebuddy", "m"},
		{"", "alicode-intl/vendor/orphan"}, {"openai", "vendor/alicode-intl/m"},
	} {
		if err := database.SaveUsageEntry(&gatewaydb.UsageEntry{Timestamp: "2026-09-01T01:00:00Z", Provider: item.provider, Model: item.model, PromptTokens: 10, CompletionTokens: 4, Cost: 0.25}); err != nil {
			t.Fatal(err)
		}
	}
	execSQL(t, database, `UPDATE usageDaily SET data = json_set(data,
		'$.extra', json('{"n":9007199254740993}'),
		'$.byProvider.alicode.sourceOnly', 'keep-source',
		'$.byProvider.alicode-cn.destinationOnly', 'keep-destination',
		'$.byProvider.alicode.conflict', 'source',
		'$.byProvider.alicode-cn.conflict', 'destination',
		'$.byModel."alicode/vendor/m|alicode".sourceOnly', 'model-source',
		'$.byModel."alicode-cn/vendor/m|alicode-cn".destinationOnly', 'model-destination')`)
	if err := migrate(database.Conn()); err != nil {
		t.Fatal(err)
	}
	day, err := database.GetUsageDaily("2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	one := gatewaydb.DayCounter{Requests: 1, PromptTokens: 10, CompletionTokens: 4, Cost: 0.25}
	two := gatewaydb.DayCounter{Requests: 2, PromptTokens: 20, CompletionTokens: 8, Cost: 0.5}
	want := gatewaydb.DayData{
		Requests: 7, PromptTokens: 70, CompletionTokens: 28, Cost: 1.75,
		ByProvider: map[string]gatewaydb.DayCounter{"alicode-cn": two, "alicode": one, "codebuddy": two, "openai": one},
		ByModel:    map[string]gatewaydb.DayCounter{"alicode-cn/vendor/m|alicode-cn": two, "alicode/vendor/m|alicode": one, "m|codebuddy": two, "alicode/vendor/orphan": one, "vendor/alicode-intl/m|openai": one},
	}
	if !reflect.DeepEqual(*day, want) {
		t.Errorf("daily = %+v, want %+v", *day, want)
	}
	raw := queryString(t, database, `SELECT data FROM usageDaily`)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, string(fields["extra"]), `{"n":9007199254740993}`)
	for path, want := range map[string]string{
		`$.byProvider.alicode-cn.sourceOnly`: "keep-source", `$.byProvider.alicode-cn.destinationOnly`: "keep-destination",
		`$.byProvider.alicode-cn.conflict`: "destination", `$.byModel."alicode-cn/vendor/m|alicode-cn".sourceOnly`: "model-source",
		`$.byModel."alicode-cn/vendor/m|alicode-cn".destinationOnly`: "model-destination",
	} {
		if got := queryString(t, database, `SELECT COALESCE(json_extract(data, ?), '') FROM usageDaily`, path); got != want {
			t.Errorf("unknown field %s = %s, want %s", path, got, want)
		}
	}
	if err := migrate(database.Conn()); err != nil {
		t.Fatal(err)
	}
	if got := queryString(t, database, `SELECT data FROM usageDaily`); got != raw {
		t.Error("second migration changed daily JSON")
	}
	payload, err := database.ExportData(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.Data.UsageHistory) != 7 || len(payload.Data.UsageDaily) != 1 {
		t.Fatalf("incomplete export: %+v", payload.Data)
	}
	target, _ := newMigrationDB(t)
	for attempt := 0; attempt < 2; attempt++ {
		if err := target.ImportData(payload, gatewaydb.RestoreModeMerge); err != nil {
			t.Fatalf("merge %d: %v", attempt, err)
		}
		got, err := target.GetUsageDaily("2026-09-01")
		if err != nil || !reflect.DeepEqual(got, &want) {
			t.Errorf("merged daily = %+v, %v", got, err)
		}
	}
	if got := queryString(t, target, `SELECT count(*) FROM usageHistory`); got != "7" {
		t.Errorf("merge duplicated history: %s", got)
	}
	if got := queryString(t, target, `SELECT value FROM _meta WHERE key = 'totalRequestsLifetime'`); got != "7" {
		t.Errorf("merged lifetime = %s", got)
	}
}

func TestMigrateDailyPreservesUntouchedDimensions(t *testing.T) {
	database, _ := newMigrationDB(t)
	seedConnectionData(t, database, "cn", "alicode", "api-key", chinaBaseURL)
	execSQL(t, database, `INSERT INTO usageDaily VALUES ('2026-09-02', '{"requests":1,"byProvider":{"alicode":{"requests":1,"extra":9007199254740993}},"byModel":null,"future":{"key":"alicode"}}')`)
	if err := migrate(database.Conn()); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, queryString(t, database, `SELECT data FROM usageDaily`), `{"requests":1,"byProvider":{"alicode-cn":{"requests":1,"extra":9007199254740993}},"byModel":null,"future":{"key":"alicode"}}`)
}

func TestMigrateLaterSQLFailureRollsBackEverything(t *testing.T) {
	database, _ := newMigrationDB(t)
	seedConnection(t, database, "cn", "alicode", "api-key")
	seedConnection(t, database, "none", "opencode", "none")
	execSQL(t, database, `INSERT INTO kv VALUES ('aliases', 'a', 'alicode-intl/m'), ('providerModelCache', 'alicode', '{"models":[]}'), ('disabledModels', 'alicode/m', 'true'), ('modelMetaOverrides', 'alicode-intl/m', '{"displayName":"Intl"}')`)
	execSQL(t, database, `INSERT INTO apiKeys (id, key, allowedModels, createdAt) VALUES ('k', 'fixture-only', '["alicode/*","codebuddy-intl*"]', '2026-09-01T00:00:00Z')`)
	execSQL(t, database, `INSERT INTO settings VALUES (1, '{"tokenSaverExclude":["alicode-intl"],"passwordHash":"fixture-only"}')`)
	execSQL(t, database, `INSERT INTO combos VALUES ('c', 'fallback', '', '["alicode-intl/m"]', '', '')`)
	execSQL(t, database, `INSERT INTO requestDetails VALUES ('r', '2026-09-01T00:00:00Z', 'alicode', 'alicode/m', 'cn', 'ok', '{"provider":"alicode","model":"alicode/m"}')`)
	if err := database.SaveUsageEntry(&gatewaydb.UsageEntry{Timestamp: "2026-09-01T00:00:00Z", Provider: "alicode", Model: "alicode/m"}); err != nil {
		t.Fatal(err)
	}
	before, err := database.ExportData(true, true)
	if err != nil {
		t.Fatal(err)
	}
	// This fails at the final write, after every data transformation.
	execSQL(t, database, `CREATE TRIGGER fail_marker BEFORE INSERT ON kv
		WHEN NEW.scope = 'standaloneMigrations' AND NEW.key = 'provider-ids-v1'
		BEGIN SELECT RAISE(ABORT, 'deliberate late failure'); END`)
	if err := migrate(database.Conn()); err == nil || !strings.Contains(err.Error(), "deliberate late failure") {
		t.Errorf("migration error = %v, want deliberate late SQL failure", err)
	}
	after, err := database.ExportData(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Data, after.Data) {
		t.Error("failed migration left partially changed data")
	}
	assertMarker(t, database, false)
	execSQL(t, database, `DROP TRIGGER fail_marker`)
	if err := migrate(database.Conn()); err != nil {
		t.Fatalf("retry: %v", err)
	}
	assertMarker(t, database, true)
}

func TestMigrationCLIFailureIsNonzero(t *testing.T) {
	database, dir := newMigrationDB(t)
	seedPreCanonical(t, database)
	seedConnection(t, database, "cn", "alicode", "api-key")
	execSQL(t, database, `DROP TABLE combos`)
	output, err := migrationCLI(dir)
	if err == nil {
		t.Errorf("CLI succeeded on missing table: %s", output)
	}
	if strings.Contains(string(output), "completed successfully") {
		t.Errorf("CLI reported success on error: %s", output)
	}
	if got := queryString(t, database, `SELECT provider FROM providerConnections WHERE id = 'cn'`); got != "alicode" {
		t.Errorf("CLI failure left provider = %s", got)
	}
	assertMarker(t, database, false)
}

// seedCanonicalShape mirrors an already-canonical production database: the
// observed provider set, a canonical alicode that means Intl, and no legacy ID.
func seedCanonicalShape(t *testing.T, database *gatewaydb.DB) {
	t.Helper()
	for _, id := range []string{"antigravity", "codebuddy", "codebuddy-cn", "qoder", "qoder-cn"} {
		seedConnectionData(t, database, id, id, "oauth", `{"apiKey":"fixture-only"}`)
	}
	seedConnectionData(t, database, "ali", "alicode", "oauth", intlBaseURL)
	seedRequestDetail(t, database, "r", "alicode", "alicode/vendor/m", "ali")
	seedUsage(t, database, "alicode", "alicode/vendor/m", "ali")
	seedUsage(t, database, "codebuddy-cn", "codebuddy-cn/vendor/m", "codebuddy-cn")
	execSQL(t, database, `INSERT INTO kv VALUES ('providerModelCache', 'alicode', '{"models":[{"id":"vendor/m"}]}'),
		('aliases', 'ali', 'alicode/vendor/m'), ('disabledModels', 'alicode/m', 'true'),
		('modelMetaOverrides', 'alicode/m', '{"displayName":"Intl"}')`)
	execSQL(t, database, `INSERT INTO combos VALUES ('c', 'fallback', '', '["alicode/vendor/m"]', '', '')`)
	execSQL(t, database, `INSERT INTO usageDaily VALUES ('2026-09-04', '{"requests":1,"byProvider":{"alicode":{"requests":1}},"byModel":{"alicode/vendor/m|alicode":{"requests":1}}}')`)
}

func TestMigrateRefusesDatabaseWithoutPreCanonicalEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra func(*testing.T, *gatewaydb.DB)
	}{
		{"canonical-ids-with-intl-alicode", nil},
		{"china-host-belongs-to-another-provider", func(t *testing.T, database *gatewaydb.DB) {
			seedConnectionData(t, database, "foreign", "codebuddy", "api-key", chinaBaseURL)
		}},
		{"similar-ids-and-paths", func(t *testing.T, database *gatewaydb.DB) {
			seedConnectionData(t, database, "similar", "alicode-intl-extra", "api-key", chinaBaseURL)
			seedRequestDetail(t, database, "similar", "openrouter", "vendor/alicode-intl/m", "similar")
			execSQL(t, database, `INSERT INTO kv VALUES ('providerModelCache', 'alicode-intl-extra', '{"models":[]}'),
				('aliases', 'similar', 'openrouter/alicode-intl/m'), ('disabledModels', 'codebuddy-intl-extra/m', 'true')`)
			execSQL(t, database, `INSERT INTO usageDaily VALUES ('2026-09-05', '{"requests":1,"byProvider":{"alicode-intl-extra":{"requests":1}},"byModel":{"openrouter/alicode-intl/m|openrouter":{"requests":1}}}')`)
			execSQL(t, database, `INSERT INTO combos VALUES ('similar', 'similar', '', '["alicode-intl-extra/m","vendor/codebuddy-intl/m"]', '', '')`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, _ := newMigrationDB(t)
			seedCanonicalShape(t, database)
			if tc.extra != nil {
				tc.extra(t, database)
			}
			before, err := database.ExportData(true, true)
			if err != nil {
				t.Fatal(err)
			}
			err = migrate(database.Conn())
			if err == nil || !strings.Contains(err.Error(), "does not look pre-canonical") {
				t.Fatalf("migrate = %v, want refusal of an already canonical database", err)
			}
			after, err := database.ExportData(true, true)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Data, after.Data) {
				t.Error("refused migration changed data")
			}
			if got := queryString(t, database, `SELECT provider FROM providerConnections WHERE id = 'ali'`); got != "alicode" {
				t.Errorf("canonical alicode became %s", got)
			}
			assertMarker(t, database, false)
		})
	}
}

func TestMigratePreCanonicalEvidenceProceeds(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		seed func(*testing.T, *gatewaydb.DB)
	}{
		{"alicode-china-host", chinaBaseURL, nil},
		{"codebuddy-intl-connection", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			seedConnection(t, database, "legacy-buddy", "codebuddy-intl", "oauth")
		}},
		{"alicode-intl-connection", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			seedConnection(t, database, "legacy-intl", "alicode-intl", "api-key")
		}},
		{"legacy-request-detail", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			seedRequestDetail(t, database, "legacy", "codebuddy-intl", "codebuddy-intl/vendor/m", "codebuddy")
		}},
		{"legacy-usage-row", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			seedUsage(t, database, "alicode-intl", "alicode-intl/vendor/m", "ali")
		}},
		{"legacy-cache-key", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			execSQL(t, database, `INSERT INTO kv VALUES ('providerModelCache', 'codebuddy-intl', '{"models":[]}')`)
		}},
		{"legacy-alias-value", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			execSQL(t, database, `INSERT INTO kv VALUES ('aliases', 'legacy', 'alicode-intl/vendor/m')`)
		}},
		{"legacy-disabled-key", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			execSQL(t, database, `INSERT INTO kv VALUES ('disabledModels', 'alicode-intl/m', 'true')`)
		}},
		{"legacy-daily-dimension", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			execSQL(t, database, `INSERT INTO usageDaily VALUES ('2026-09-06', '{"requests":1,"byProvider":{"codebuddy-intl":{"requests":1}},"byModel":{"codebuddy-intl/vendor/m|codebuddy-intl":{"requests":1}}}')`)
		}},
		{"legacy-combo-model", intlBaseURL, func(t *testing.T, database *gatewaydb.DB) {
			execSQL(t, database, `INSERT INTO combos VALUES ('legacy', 'legacy', '', '["codebuddy-intl/vendor/m"]', '', '')`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			database, _ := newMigrationDB(t)
			seedCanonicalShape(t, database)
			if tc.data != intlBaseURL {
				execSQL(t, database, `UPDATE providerConnections SET data = ? WHERE id = 'ali'`, tc.data)
			}
			if tc.seed != nil {
				tc.seed(t, database)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if err := migrate(database.Conn()); err != nil {
					t.Fatalf("attempt %d: %v", attempt, err)
				}
			}
			assertMarker(t, database, true)
			if got := queryString(t, database, `SELECT count(*) FROM providerConnections WHERE provider IN ('alicode-intl','codebuddy-intl')`); got != "0" {
				t.Errorf("legacy connections left: %s", got)
			}
			for query, want := range map[string]string{
				`SELECT provider FROM providerConnections WHERE id = 'ali'`:                                                      "alicode-cn",
				`SELECT model FROM requestDetails WHERE id = 'r'`:                                                                "alicode-cn/vendor/m",
				`SELECT count(*) FROM usageHistory WHERE provider = 'alicode-cn'`:                                                "1",
				`SELECT count(*) FROM kv WHERE scope = 'disabledModels' AND key = 'alicode-cn/m'`:                                "1",
				`SELECT json_extract(value, '$.models[0].id') FROM kv WHERE scope = 'providerModelCache' AND key = 'alicode-cn'`: "vendor/m",
				`SELECT value FROM kv WHERE scope = 'aliases' AND key = 'ali'`:                                                   "alicode-cn/vendor/m",
				`SELECT json_extract(data, '$.byProvider."alicode-cn".requests') FROM usageDaily WHERE dateKey = '2026-09-04'`:   "1",
				`SELECT models FROM combos WHERE id = 'c'`:                                                                       `["alicode-cn/vendor/m"]`,
			} {
				if got := queryString(t, database, query); got != want {
					t.Errorf("%s = %q, want %q", query, got, want)
				}
			}
		})
	}
}

func assertMarker(t *testing.T, database *gatewaydb.DB, want bool) {
	t.Helper()
	var value string
	err := database.Conn().QueryRow(`SELECT value FROM kv WHERE scope = 'standaloneMigrations' AND key = 'provider-ids-v1'`).Scan(&value)
	if (want && err != nil) || (!want && err != sql.ErrNoRows) {
		t.Errorf("marker present = %v, want %v (error: %v)", err == nil, want, err)
	}
}

func assertJSON(t *testing.T, got, want string) {
	t.Helper()
	decode := func(raw string) any {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			t.Fatalf("decode %q: %v", raw, err)
		}
		return value
	}
	if !reflect.DeepEqual(decode(got), decode(want)) {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
