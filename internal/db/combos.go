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

	var combos []model.Combo
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

func (d *DB) DeleteCombo(id string) error {
	_, err := d.conn.Exec(`DELETE FROM combos WHERE id = ?`, id)
	return err
}

// API Keys repository

func (d *DB) ListAPIKeys() ([]model.APIKey, error) {
	rows, err := d.conn.Query(`SELECT id, key, name, machineId, isActive, createdAt FROM apiKeys ORDER BY createdAt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []model.APIKey
	for rows.Next() {
		var k model.APIKey
		var name, machineID sql.NullString
		var isActive int
		var createdAt string

		if err := rows.Scan(&k.ID, &k.Key, &name, &machineID, &isActive, &createdAt); err != nil {
			return nil, err
		}

		k.Name = name.String
		k.MachineID = machineID.String
		k.IsActive = isActive == 1
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (d *DB) ValidateAPIKey(key string) (bool, error) {
	var isActive int
	err := d.conn.QueryRow(`SELECT isActive FROM apiKeys WHERE key = ?`, key).Scan(&isActive)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isActive == 1, nil
}

func (d *DB) CreateAPIKey(k *model.APIKey) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.conn.Exec(
		`INSERT INTO apiKeys (id, key, name, machineId, isActive, createdAt) VALUES (?, ?, ?, ?, ?, ?)`,
		k.ID, k.Key, k.Name, k.MachineID, boolToInt(k.IsActive), now,
	)
	return err
}

func (d *DB) DeleteAPIKey(id string) error {
	_, err := d.conn.Exec(`DELETE FROM apiKeys WHERE id = ?`, id)
	return err
}
