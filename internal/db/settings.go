package db

import (
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrNotFound = errors.New("not found")

// Settings repository - single-row JSON blob store

type Settings struct {
	RequireLogin    bool   `json:"requireLogin"`
	RequireAPIKey   bool   `json:"requireApiKey"`
	PasswordHash    string `json:"passwordHash,omitempty"`
	ComboStrategy   string `json:"comboStrategy,omitempty"`
	RTKEnabled      bool   `json:"rtkEnabled"`
	CavemanEnabled  bool   `json:"cavemanEnabled"`
	CavemanLevel    string `json:"cavemanLevel,omitempty"`
	PonytailEnabled bool   `json:"ponytailEnabled"`
	PonytailLevel   string `json:"ponytailLevel,omitempty"`
}

func DefaultSettings() *Settings {
	return &Settings{
		RequireLogin:  false,
		RequireAPIKey: false,
		ComboStrategy: "fallback",
	}
}

func (d *DB) GetSettings() (*Settings, error) {
	var data string
	err := d.conn.QueryRow(`SELECT data FROM settings WHERE id = 1`).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return nil, err
	}

	s := DefaultSettings()
	if err := json.Unmarshal([]byte(data), s); err != nil {
		return nil, err
	}
	return s, nil
}

func (d *DB) SaveSettings(s *Settings) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(
		`INSERT INTO settings (id, data) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		string(data),
	)
	return err
}

// KV store

func (d *DB) KVGet(scope, key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM kv WHERE scope = ? AND key = ?`, scope, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

func (d *DB) KVSet(scope, key, value string) error {
	_, err := d.conn.Exec(
		`INSERT INTO kv (scope, key, value) VALUES (?, ?, ?) ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
		scope, key, value,
	)
	return err
}

func (d *DB) KVDelete(scope, key string) error {
	_, err := d.conn.Exec(`DELETE FROM kv WHERE scope = ? AND key = ?`, scope, key)
	return err
}

func (d *DB) KVList(scope string) (map[string]string, error) {
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
	return result, rows.Err()
}

// Meta store

func (d *DB) MetaGet(key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM _meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

func (d *DB) MetaSet(key, value string) error {
	_, err := d.conn.Exec(
		`INSERT INTO _meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
