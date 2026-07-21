package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Provider Nodes repository (custom compatible endpoints)

func (d *DB) ListNodes() ([]model.ProviderNode, error) {
	rows, err := d.conn.Query(
		`SELECT id, type, name, data, createdAt, updatedAt FROM providerNodes ORDER BY createdAt ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.ProviderNode
	for rows.Next() {
		var n model.ProviderNode
		var data string
		var typ, name sql.NullString
		var createdAt, updatedAt string

		if err := rows.Scan(&n.ID, &typ, &name, &data, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		n.Type = typ.String
		n.Name = name.String
		n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if err := json.Unmarshal([]byte(data), &n.Data); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (d *DB) GetNode(id string) (*model.ProviderNode, error) {
	var n model.ProviderNode
	var data string
	var typ, name sql.NullString
	var createdAt, updatedAt string

	err := d.conn.QueryRow(
		`SELECT id, type, name, data, createdAt, updatedAt FROM providerNodes WHERE id = ?`, id,
	).Scan(&n.ID, &typ, &name, &data, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	n.Type = typ.String
	n.Name = name.String
	n.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	n.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if err := json.Unmarshal([]byte(data), &n.Data); err != nil {
		return nil, err
	}
	return &n, nil
}

func (d *DB) CreateNode(n *model.ProviderNode) error {
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(n.Data)
	if err != nil {
		return err
	}

	_, err = d.conn.Exec(
		`INSERT INTO providerNodes (id, type, name, data, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
		n.ID, n.Type, n.Name, string(data), now, now,
	)
	return err
}

func (d *DB) UpdateNode(n *model.ProviderNode) error {
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(n.Data)
	if err != nil {
		return err
	}

	_, err = d.conn.Exec(
		`UPDATE providerNodes SET type=?, name=?, data=?, updatedAt=? WHERE id=?`,
		n.Type, n.Name, string(data), now, n.ID,
	)
	return err
}

func (d *DB) DeleteNode(id string) error {
	_, err := d.conn.Exec(`DELETE FROM providerNodes WHERE id = ?`, id)
	return err
}
