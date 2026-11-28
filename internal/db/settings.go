package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"slices"
)

var ErrNotFound = errors.New("not found")

// Settings repository - single-row JSON blob store

type Settings struct {
	PasswordHash  string `json:"passwordHash,omitempty"`
	ComboStrategy string `json:"comboStrategy,omitempty"`
	RTKLevel      string `json:"rtkLevel,omitempty"`
	CavemanLevel  string `json:"cavemanLevel,omitempty"`
	PonytailLevel string `json:"ponytailLevel,omitempty"`
	// TokenSaverExclude lists providers where caveman/ponytail breaks output (9router#2767).
	TokenSaverExclude []string `json:"tokenSaverExclude,omitempty"`
	// APIKeyRPM caps requests per API key per minute for /v1/* calls.
	// 0 disables inbound rate limiting (default, backwards compatible).
	APIKeyRPM            int  `json:"apiKeyRpm,omitempty"`
	ResponseCacheTTL     int  `json:"responseCacheTTL,omitempty"`
	HasPassword          bool `json:"hasPassword"`
	RequireLogin         bool `json:"requireLogin"`
	RequireAPIKey        bool `json:"requireApiKey"`
	RTKEnabled           bool `json:"rtkEnabled"`
	CavemanEnabled       bool `json:"cavemanEnabled"`
	PonytailEnabled      bool `json:"ponytailEnabled"`
	LoopGuardEnabled     bool `json:"loopGuardEnabled"`
	ResponseCacheEnabled bool `json:"responseCacheEnabled"`
	ResponseCacheAll     bool `json:"responseCacheAll,omitempty"`
	AutoPromptCache      bool `json:"autoPromptCache"`
}

func cloneSettings(s *Settings) *Settings {
	if s == nil {
		return nil
	}
	cp := *s
	if s.TokenSaverExclude != nil {
		cp.TokenSaverExclude = slices.Clone(s.TokenSaverExclude)
	}
	return &cp
}

func DefaultSettings() *Settings {
	return &Settings{
		RequireLogin:     false,
		RequireAPIKey:    false,
		ComboStrategy:    "fallback",
		RTKLevel:         "aggressive",
		CavemanLevel:     "lite",
		PonytailLevel:    "lite",
		LoopGuardEnabled: true,
		ResponseCacheTTL: 3600,
		AutoPromptCache:  false,
	}
}

func (d *DB) GetSettings() (*Settings, error) {
	d.cacheMu.RLock()
	cached := d.settingsCache
	d.cacheMu.RUnlock()
	if cached != nil {
		return cloneSettings(cached), nil
	}

	var data string
	err := d.conn.QueryRow(`SELECT data FROM settings WHERE id = 1`).Scan(&data)
	s := DefaultSettings()
	if !errors.Is(err, sql.ErrNoRows) {
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), s); err != nil {
			return nil, err
		}
	}

	d.cacheMu.Lock()
	d.settingsCache = cloneSettings(s)
	d.cacheMu.Unlock()
	return s, nil
}

func (d *DB) SaveSettings(s *Settings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if _, err = d.conn.Exec(
		`INSERT INTO settings (id, data) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		string(data),
	); err != nil {
		return err
	}
	d.cacheMu.Lock()
	d.settingsCache = cloneSettings(s)
	d.cacheMu.Unlock()
	return nil
}

// KV store

func (d *DB) KVGet(scope, key string) (string, error) {
	d.cacheMu.RLock()
	if d.kvScopeCache != nil {
		if scoped, ok := d.kvScopeCache[scope]; ok {
			val, found := scoped[key]
			d.cacheMu.RUnlock()
			if !found {
				return "", ErrNotFound
			}
			return val, nil
		}
	}
	d.cacheMu.RUnlock()

	var value string
	err := d.conn.QueryRow(`SELECT value FROM kv WHERE scope = ? AND key = ?`, scope, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

func (d *DB) KVSet(scope, key, value string) error {
	if _, err := d.conn.Exec(
		`INSERT INTO kv (scope, key, value) VALUES (?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
		scope, key, value,
	); err != nil {
		return err
	}
	d.invalidateKVScope(scope)
	return nil
}

func (d *DB) KVDelete(scope, key string) error {
	if _, err := d.conn.Exec(`DELETE FROM kv WHERE scope = ? AND key = ?`, scope, key); err != nil {
		return err
	}
	d.invalidateKVScope(scope)
	return nil
}

func (d *DB) KVBatchSet(scope string, keys []string, value string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO kv (scope, key, value) VALUES (?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range keys {
		if _, err := stmt.Exec(scope, k, value); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	d.invalidateKVScope(scope)
	return nil
}

func (d *DB) KVBatchDelete(scope string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`DELETE FROM kv WHERE scope = ? AND key = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, k := range keys {
		if _, err := stmt.Exec(scope, k); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	d.invalidateKVScope(scope)
	return nil
}

func (d *DB) KVList(scope string) (map[string]string, error) {
	d.cacheMu.RLock()
	if d.kvScopeCache != nil {
		if cached, ok := d.kvScopeCache[scope]; ok {
			out := maps.Clone(cached)
			d.cacheMu.RUnlock()
			return out, nil
		}
	}
	d.cacheMu.RUnlock()

	rows, err := d.conn.Query(`SELECT key, value FROM kv WHERE scope = ?`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	d.cacheMu.Lock()
	if d.kvScopeCache == nil {
		d.kvScopeCache = make(map[string]map[string]string)
	}
	d.kvScopeCache[scope] = maps.Clone(result)
	d.cacheMu.Unlock()
	return result, nil
}
