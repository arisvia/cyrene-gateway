package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gatewaydb "github.com/arisvia/cyrene-gateway/internal/db"
)

func main() {
	var dataDir string
	flag.StringVar(&dataDir, "data-dir", "", "pre-canonical data directory (defaults to ~/.cyrene-gateway); stop the gateway and back up first; unmarked alicode means China, not current Intl; databases without legacy IDs are refused")
	flag.Parse()

	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("failed to resolve user home dir: %v", err)
		}
		dataDir = filepath.Join(home, ".cyrene-gateway")
	}

	dbPath := filepath.Join(dataDir, "data.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		log.Fatalf("cannot access database at %s: %v", dbPath, err)
	}

	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	fmt.Printf("Running data migration on %s...\n", dbPath)
	if err := migrate(database); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Data migration completed successfully.")
}

// providerRenames maps offline pre-canonical IDs; unmarked alicode means China, never current Intl.
var providerRenames = []struct{ from, to string }{
	{"alicode", "alicode-cn"}, // Move ORIGINAL China before giving Intl this ID.
	{"alicode-intl", "alicode"},
	{"codebuddy-intl", "codebuddy"},
}

// preCanonicalProviderIDs lists IDs matchable before canonicalization; alicode-cn and bare codebuddy are migration output.
var preCanonicalProviderIDs = []string{"alicode", "alicode-intl", "codebuddy-cn", "codebuddy-intl"}

// preCanonicalEvidenceQuery is the sole migration gate: legacy Intl IDs or an alicode China host, always with ID boundaries.
const preCanonicalEvidenceQuery = `
	SELECT EXISTS (
		SELECT 1 FROM providerConnections WHERE provider IN ('alicode-intl', 'codebuddy-intl')
		UNION ALL SELECT 1 FROM requestDetails
			WHERE provider IN ('alicode-intl', 'codebuddy-intl')
				OR model LIKE 'alicode-intl/%' OR model LIKE 'codebuddy-intl/%'
				OR json_extract(data, '$.provider') IN ('alicode-intl', 'codebuddy-intl')
				OR json_extract(data, '$.model') LIKE 'alicode-intl/%'
				OR json_extract(data, '$.model') LIKE 'codebuddy-intl/%'
		UNION ALL SELECT 1 FROM usageHistory
			WHERE provider IN ('alicode-intl', 'codebuddy-intl')
				OR model LIKE 'alicode-intl/%' OR model LIKE 'codebuddy-intl/%'
		UNION ALL SELECT 1 FROM usageDaily, json_each(COALESCE(json_extract(usageDaily.data, '$.byProvider'), '{}'))
			WHERE key IN ('alicode-intl', 'codebuddy-intl')
		UNION ALL SELECT 1 FROM usageDaily, json_each(COALESCE(json_extract(usageDaily.data, '$.byModel'), '{}'))
			WHERE key LIKE 'alicode-intl/%' OR key LIKE 'codebuddy-intl/%'
				OR key LIKE '%|alicode-intl' OR key LIKE '%|codebuddy-intl'
		UNION ALL SELECT 1 FROM kv
			WHERE key IN ('alicode-intl', 'codebuddy-intl')
				OR (scope = 'aliases' AND (value IN ('alicode-intl', 'codebuddy-intl')
					OR value LIKE 'alicode-intl/%' OR value LIKE 'codebuddy-intl/%'))
				OR (scope IN ('disabledModels', 'modelMetaOverrides')
					AND (key LIKE 'alicode-intl/%' OR key LIKE 'codebuddy-intl/%'))
		UNION ALL SELECT 1 FROM combos, json_each(COALESCE(combos.models, '[]'))
			WHERE value IN ('alicode-intl', 'codebuddy-intl')
				OR value LIKE 'alicode-intl/%' OR value LIKE 'codebuddy-intl/%'
	) OR EXISTS (
		SELECT 1 FROM providerConnections
			WHERE provider = 'alicode' AND json_valid(data)
				AND COALESCE(json_extract(data, '$.baseUrl'), '') LIKE '%coding.dashscope.aliyuncs.com%'
	)`

func migrate(database *sql.DB) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()

	var marker string
	err = tx.QueryRow(`SELECT value FROM kv WHERE scope = 'standaloneMigrations' AND key = 'provider-ids-v1'`).Scan(&marker)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("read migration marker: %w", err)
	}

	evidence, err := hasPreCanonicalEvidence(tx)
	if err != nil {
		return fmt.Errorf("read pre-canonical evidence: %w", err)
	}
	if !evidence {
		return errors.New("database does not look pre-canonical: no alicode-intl/codebuddy-intl reference and no alicode China endpoint; " +
			"it is already canonical, so it must not be migrated")
	}

	for _, rename := range providerRenames {
		if err := renameReferences(tx, rename.from, rename.to); err != nil {
			return fmt.Errorf("rename %s -> %s: %w", rename.from, rename.to, err)
		}
	}
	if err := migrateModelKeys(tx); err != nil {
		return fmt.Errorf("migrate model keys: %w", err)
	}
	if err := migrateCombos(tx); err != nil {
		return fmt.Errorf("migrate combos: %w", err)
	}
	if err := migrateAPIAllowlists(tx); err != nil {
		return fmt.Errorf("migrate API allowlists: %w", err)
	}
	if err := migrateTokenSaverExclusions(tx); err != nil {
		return fmt.Errorf("migrate token saver exclusions: %w", err)
	}
	if err := migrateUsageDaily(tx); err != nil {
		return fmt.Errorf("migrate usageDaily: %w", err)
	}
	if _, err := tx.Exec(`UPDATE providerConnections SET authType = 'api-key', isActive = 0 WHERE provider = 'opencode' AND authType = 'none'`); err != nil {
		return fmt.Errorf("deactivate OpenCode none auth: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('standaloneMigrations', 'provider-ids-v1', '1')`); err != nil {
		return fmt.Errorf("write migration marker: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func hasPreCanonicalEvidence(tx *sql.Tx) (bool, error) {
	var found int
	if err := tx.QueryRow(preCanonicalEvidenceQuery).Scan(&found); err != nil {
		return false, err
	}
	return found != 0, nil
}

func renameReferences(tx *sql.Tx, from, to string) error {
	for _, table := range []string{"providerConnections", "requestDetails", "usageHistory"} {
		if _, err := tx.Exec(`UPDATE `+table+` SET provider = ? WHERE provider = ?`, to, from); err != nil {
			return fmt.Errorf("%s provider: %w", table, err)
		}
	}
	prefix := from + "/"
	for _, table := range []string{"requestDetails", "usageHistory"} {
		if _, err := tx.Exec(`UPDATE `+table+` SET model = ? || substr(model, ?) WHERE substr(model, 1, ?) = ?`,
			to+"/", len(prefix)+1, len(prefix), prefix); err != nil {
			return fmt.Errorf("%s model: %w", table, err)
		}
	}
	// The UI reads provider/model from the JSON, so opaque fields such as prompts stay untouched.
	if _, err := tx.Exec(`UPDATE requestDetails SET data = json_set(data, '$.provider', ?) WHERE json_extract(data, '$.provider') = ?`, to, from); err != nil {
		return fmt.Errorf("requestDetails JSON provider: %w", err)
	}
	if _, err := tx.Exec(`UPDATE requestDetails SET data = json_set(data, '$.model', ? || substr(json_extract(data, '$.model'), ?))
		WHERE substr(json_extract(data, '$.model'), 1, ?) = ?`, to+"/", len(prefix)+1, len(prefix), prefix); err != nil {
		return fmt.Errorf("requestDetails JSON model: %w", err)
	}
	if _, err := tx.Exec(`UPDATE kv SET value = ? || substr(value, ?) WHERE scope = 'aliases' AND substr(value, 1, ?) = ?`,
		to+"/", len(prefix)+1, len(prefix), prefix); err != nil {
		return fmt.Errorf("aliases: %w", err)
	}
	// Cache policy: an existing canonical destination wins; only its primary-key collision is ignored, other failures abort.
	if _, err := tx.Exec(`INSERT INTO kv (scope, key, value)
		SELECT scope, ?, value FROM kv WHERE scope = 'providerModelCache' AND key = ?
		ON CONFLICT(scope, key) DO NOTHING`, to, from); err != nil {
		return fmt.Errorf("copy providerModelCache: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM kv WHERE scope = 'providerModelCache' AND key = ?`, from); err != nil {
		return fmt.Errorf("remove legacy providerModelCache: %w", err)
	}
	return nil
}

func renameProvider(value string) string {
	for _, rename := range providerRenames {
		if value == rename.from {
			return rename.to
		}
	}
	return value
}

func renameModel(value string) string {
	if prefix, suffix, ok := strings.Cut(value, "/"); ok {
		return renameProvider(prefix) + "/" + suffix
	}
	return value
}

func renameDailyModel(value string) string {
	if index := strings.LastIndexByte(value, '|'); index >= 0 {
		return renameModel(value[:index]) + "|" + renameProvider(value[index+1:])
	}
	return renameModel(value)
}

// Materialize rows before writes on the transaction's single connection.
func readTextPairs(tx *sql.Tx, query string, args ...any) ([][2]string, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pairs [][2]string
	for rows.Next() {
		var pair [2]string
		if err := rows.Scan(&pair[0], &pair[1]); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair)
	}
	return pairs, rows.Err()
}

func migrateModelKeys(tx *sql.Tx) error {
	for _, scope := range []string{"disabledModels", "modelMetaOverrides"} {
		rows, err := readTextPairs(tx, `SELECT key, value FROM kv WHERE scope = ?`, scope)
		if err != nil {
			return err
		}
		// Remove original keys first so China's old ID cannot swallow Intl's value.
		for _, row := range rows {
			if renameModel(row[0]) != row[0] {
				if _, err := tx.Exec(`DELETE FROM kv WHERE scope = ? AND key = ?`, scope, row[0]); err != nil {
					return err
				}
			}
		}
		conflict := `DO NOTHING`
		if scope == "disabledModels" {
			conflict = `DO UPDATE SET value = 'true'`
		}
		// Restrictions win collisions; opaque metadata keeps the canonical destination.
		for _, row := range rows {
			target := renameModel(row[0])
			if target == row[0] {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO kv (scope, key, value) VALUES (?, ?, ?) ON CONFLICT(scope, key) `+conflict, scope, target, row[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func transformStringArray(raw string, transform func(string) []string) (string, error) {
	var original []string
	if err := json.Unmarshal([]byte(raw), &original); err != nil {
		return "", err
	}
	var result []string
	for _, value := range original {
		result = append(result, transform(value)...)
	}
	if slices.Equal(original, result) {
		return raw, nil
	}
	encoded, err := json.Marshal(result)
	return string(encoded), err
}

func migrateCombos(tx *sql.Tx) error {
	rows, err := readTextPairs(tx, `SELECT id, models FROM combos`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		raw, err := transformStringArray(row[1], func(value string) []string { return []string{renameModel(value)} })
		if err != nil {
			return fmt.Errorf("combo %s: %w", row[0], err)
		}
		if raw == row[1] {
			continue
		}
		if _, err := tx.Exec(`UPDATE combos SET models = ? WHERE id = ?`, raw, row[0]); err != nil {
			return err
		}
	}
	return nil
}

func renameFoldedReference(value string) string {
	prefix, _, _ := strings.Cut(strings.TrimSpace(value), "/")
	lower := strings.ToLower(prefix)
	if target := renameProvider(lower); target != lower {
		return strings.Replace(value, prefix, target, 1)
	}
	return value
}

func renameAllowedPattern(value string) []string {
	pattern := strings.TrimSpace(value)
	if strings.Contains(pattern, "/") {
		if strings.HasSuffix(pattern, "*") {
			return []string{strings.Replace(value, pattern, renameModel(pattern), 1)}
		}
		return []string{renameFoldedReference(value)}
	}
	prefix, wildcard := strings.CutSuffix(pattern, "*")
	if !wildcard {
		return []string{value}
	}
	entries, ok := mappedRegionWildcards(prefix)
	if !ok {
		return []string{value}
	}
	return entries
}

// mappedRegionWildcards resolves a wildcard prefix to pre-canonical IDs, dropping the original so it cannot match alicode-cn.
func mappedRegionWildcards(prefix string) ([]string, bool) {
	if prefix == "" {
		return nil, false
	}
	var entries []string
	renamed := false
	for _, id := range preCanonicalProviderIDs {
		// IsModelAllowed matches raw wildcard prefixes case-sensitively.
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		target := renameProvider(id)
		renamed = renamed || target != id
		if entry := target + "/*"; !slices.Contains(entries, entry) {
			entries = append(entries, entry)
		}
	}
	if !renamed {
		return nil, false
	}
	return entries, true
}

func migrateAPIAllowlists(tx *sql.Tx) error {
	rows, err := readTextPairs(tx, `SELECT id, allowedModels FROM apiKeys WHERE allowedModels IS NOT NULL AND allowedModels != ''`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		raw, err := transformStringArray(row[1], renameAllowedPattern)
		if err != nil {
			return fmt.Errorf("API key %s: %w", row[0], err)
		}
		if raw == row[1] {
			continue
		}
		if _, err := tx.Exec(`UPDATE apiKeys SET allowedModels = ? WHERE id = ?`, raw, row[0]); err != nil {
			return err
		}
	}
	return nil
}

func migrateTokenSaverExclusions(tx *sql.Tx) error {
	rows, err := readTextPairs(tx, `SELECT id, data FROM settings WHERE id = 1`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var settings map[string]json.RawMessage
		if err := json.Unmarshal([]byte(row[1]), &settings); err != nil {
			return err
		}
		exclusions, ok := settings["tokenSaverExclude"]
		if !ok {
			continue
		}
		raw, err := transformStringArray(string(exclusions), func(value string) []string { return []string{renameFoldedReference(value)} })
		if err != nil {
			return err
		}
		if raw == string(exclusions) {
			continue
		}
		if _, err := tx.Exec(`UPDATE settings SET data = json_set(data, '$.tokenSaverExclude', json(?)) WHERE id = 1`, raw); err != nil {
			return err
		}
	}
	return nil
}

func migrateUsageDaily(tx *sql.Tx) error {
	rows, err := readTextPairs(tx, `SELECT dateKey, data FROM usageDaily`)
	if err != nil {
		return err
	}
	for _, row := range rows {
		var day map[string]json.RawMessage
		if err := json.Unmarshal([]byte(row[1]), &day); err != nil {
			return fmt.Errorf("day %s: %w", row[0], err)
		}
		changed := false
		for _, field := range []string{"byProvider", "byModel"} {
			if _, ok := day[field]; !ok {
				continue
			}
			var dimension map[string]json.RawMessage
			if err := json.Unmarshal(day[field], &dimension); err != nil {
				return fmt.Errorf("day %s %s: %w", row[0], field, err)
			}
			rename := renameProvider
			if field == "byModel" {
				rename = renameDailyModel
			}
			merged := make(map[string]json.RawMessage, len(dimension))
			// Seed unchanged destinations first so their unknown fields win on collision.
			for key, value := range dimension {
				if rename(key) == key {
					merged[key] = value
				}
			}
			dimensionChanged := false
			// Transform original keys once; never re-transform the newly created IDs.
			for _, key := range slices.Sorted(maps.Keys(dimension)) {
				target := rename(key)
				if target == key {
					continue
				}
				value := dimension[key]
				if destination, ok := merged[target]; ok {
					value, err = mergeDayCounters(destination, value)
					if err != nil {
						return fmt.Errorf("day %s %s %s: %w", row[0], field, key, err)
					}
				}
				merged[target] = value
				dimensionChanged = true
			}
			if dimensionChanged {
				day[field], err = json.Marshal(merged)
				if err != nil {
					return err
				}
				changed = true
			}
		}
		if !changed {
			continue
		}
		// Top-level totals and unknown fields remain raw, including large numbers.
		raw, err := json.Marshal(day)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE usageDaily SET data = ? WHERE dateKey = ?`, string(raw), row[0]); err != nil {
			return err
		}
	}
	return nil
}

func mergeDayCounters(destination, source json.RawMessage) (json.RawMessage, error) {
	var dst, src gatewaydb.DayCounter
	if err := json.Unmarshal(destination, &dst); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(source, &src); err != nil {
		return nil, err
	}
	dst.Requests += src.Requests
	dst.PromptTokens += src.PromptTokens
	dst.CompletionTokens += src.CompletionTokens
	dst.Cost += src.Cost
	summedCounters, err := json.Marshal(dst)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]json.RawMessage)
	// Sequential unmarshal order is the field policy: source fills gaps, destination wins, counters overwrite.
	for _, raw := range []json.RawMessage{source, destination, summedCounters} {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}
