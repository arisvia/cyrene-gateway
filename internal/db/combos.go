package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Combos repository

func (d *DB) ListCombos() ([]model.Combo, error) {
	rows, err := d.conn.Query(`SELECT id, name, kind, models, createdAt, updatedAt FROM combos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	combos := []model.Combo{}
	for rows.Next() {
		var c model.Combo
		var models string
		var kind sql.NullString
		var createdAt, updatedAt string

		if err := rows.Scan(&c.ID, &c.Name, &kind, &models, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		c.Kind = kind.String
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if err := json.Unmarshal([]byte(models), &c.Models); err != nil {
			return nil, err
		}
		combos = append(combos, c)
	}
	return combos, rows.Err()
}

func (d *DB) GetComboByName(name string) (*model.Combo, error) {
	var c model.Combo
	var models string
	var kind sql.NullString
	var createdAt, updatedAt string

	err := d.conn.QueryRow(
		`SELECT id, name, kind, models, createdAt, updatedAt FROM combos WHERE name = ?`, name,
	).Scan(&c.ID, &c.Name, &kind, &models, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	c.Kind = kind.String
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if err := json.Unmarshal([]byte(models), &c.Models); err != nil {
		return nil, err
	}
	return &c, nil
}

func (d *DB) CreateCombo(c *model.Combo) error {
	now := time.Now().UTC().Format(time.RFC3339)
	models, err := json.Marshal(c.Models)
	if err != nil {
		return err
	}

	_, err = d.conn.Exec(
		`INSERT INTO combos (id, name, kind, models, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Kind, string(models), now, now,
	)
	return err
}

func (d *DB) UpdateCombo(c *model.Combo) error {
	now := time.Now().UTC().Format(time.RFC3339)
	models, err := json.Marshal(c.Models)
	if err != nil {
		return err
	}

	_, err = d.conn.Exec(
		`UPDATE combos SET name = ?, kind = ?, models = ?, updatedAt = ? WHERE id = ?`,
		c.Name, c.Kind, string(models), now, c.ID,
	)
	return err
}

func (d *DB) DeleteCombo(id string) error {
	_, err := d.conn.Exec(`DELETE FROM combos WHERE id = ?`, id)
	return err
}

// API Keys repository

func (d *DB) ListAPIKeys() ([]model.APIKey, error) {
	rows, err := d.conn.Query(`SELECT id, key, name, machineId, isActive, allowedModels, rpm, systemPrompt, expiresAt, createdAt FROM apiKeys ORDER BY createdAt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []model.APIKey{}
	for rows.Next() {
		var k model.APIKey
		var name, machineID, allowedModels, systemPrompt, expiresAt sql.NullString
		var isActive, rpm int
		var createdAt string

		if err := rows.Scan(&k.ID, &k.Key, &name, &machineID, &isActive, &allowedModels, &rpm, &systemPrompt, &expiresAt, &createdAt); err != nil {
			return nil, err
		}

		k.Name = name.String
		k.MachineID = machineID.String
		k.IsActive = isActive == 1
		k.RPM = rpm
		k.SystemPrompt = systemPrompt.String
		k.ExpiresAt = expiresAt.String
		if allowedModels.String != "" {
			_ = json.Unmarshal([]byte(allowedModels.String), &k.AllowedModels)
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (d *DB) GetAPIKey(id string) (*model.APIKey, error) {
	var k model.APIKey
	var name, machineID, allowedModels, systemPrompt, expiresAt sql.NullString
	var isActive, rpm int
	var createdAt string

	err := d.conn.QueryRow(
		`SELECT id, key, name, machineId, isActive, allowedModels, rpm, systemPrompt, expiresAt, createdAt FROM apiKeys WHERE id = ?`,
		id,
	).Scan(&k.ID, &k.Key, &name, &machineID, &isActive, &allowedModels, &rpm, &systemPrompt, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	k.Name = name.String
	k.MachineID = machineID.String
	k.IsActive = isActive == 1
	k.RPM = rpm
	k.SystemPrompt = systemPrompt.String
	k.ExpiresAt = expiresAt.String
	if allowedModels.String != "" {
		_ = json.Unmarshal([]byte(allowedModels.String), &k.AllowedModels)
	}
	k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &k, nil
}

func (d *DB) GetAPIKeyByKey(key string) (*model.APIKey, error) {
	var k model.APIKey
	var name, machineID, allowedModels, systemPrompt, expiresAt sql.NullString
	var isActive, rpm int
	var createdAt string

	err := d.conn.QueryRow(
		`SELECT id, key, name, machineId, isActive, allowedModels, rpm, systemPrompt, expiresAt, createdAt FROM apiKeys WHERE key = ?`,
		key,
	).Scan(&k.ID, &k.Key, &name, &machineID, &isActive, &allowedModels, &rpm, &systemPrompt, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	k.Name = name.String
	k.MachineID = machineID.String
	k.IsActive = isActive == 1
	k.RPM = rpm
	k.SystemPrompt = systemPrompt.String
	k.ExpiresAt = expiresAt.String
	if allowedModels.String != "" {
		_ = json.Unmarshal([]byte(allowedModels.String), &k.AllowedModels)
	}
	k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &k, nil
}

func (d *DB) ValidateAPIKey(key string) (bool, error) {
	k, err := d.GetAPIKeyByKey(key)
	if err != nil || k == nil {
		return false, err
	}
	if !k.IsActive || k.IsExpired() {
		return false, nil
	}
	return true, nil
}

func (d *DB) CreateAPIKey(k *model.APIKey) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var allowedModelsJSON string
	if len(k.AllowedModels) > 0 {
		b, _ := json.Marshal(k.AllowedModels)
		allowedModelsJSON = string(b)
	}
	_, err := d.conn.Exec(
		`INSERT INTO apiKeys (id, key, name, machineId, isActive, allowedModels, rpm, systemPrompt, expiresAt, createdAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Key, k.Name, k.MachineID, boolToInt(k.IsActive), allowedModelsJSON, k.RPM, k.SystemPrompt, k.ExpiresAt, now,
	)
	return err
}

func (d *DB) UpdateAPIKey(k *model.APIKey) error {
	var allowedModelsJSON string
	if len(k.AllowedModels) > 0 {
		b, _ := json.Marshal(k.AllowedModels)
		allowedModelsJSON = string(b)
	}
	_, err := d.conn.Exec(
		`UPDATE apiKeys SET name = ?, isActive = ?, allowedModels = ?, rpm = ?, systemPrompt = ?, expiresAt = ? WHERE id = ?`,
		k.Name, boolToInt(k.IsActive), allowedModelsJSON, k.RPM, k.SystemPrompt, k.ExpiresAt, k.ID,
	)
	return err
}

func (d *DB) DeleteAPIKey(id string) error {
	_, err := d.conn.Exec(`DELETE FROM apiKeys WHERE id = ?`, id)
	return err
}
