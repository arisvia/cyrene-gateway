package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Provider Connections repository

func (d *DB) ListConnections() ([]model.ProviderConnection, error) {
	rows, err := d.conn.Query(
		`SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt FROM providerConnections ORDER BY priority ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []model.ProviderConnection
	for rows.Next() {
		var c model.ProviderConnection
		var data string
		var name, email sql.NullString
		var priority sql.NullInt64
		var isActive int
		var createdAt, updatedAt string

		if err := rows.Scan(&c.ID, &c.Provider, &c.AuthType, &name, &email, &priority, &isActive, &data, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		c.Name = name.String
		c.Email = email.String
		c.Priority = int(priority.Int64)
		c.IsActive = isActive == 1
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if err := json.Unmarshal([]byte(data), &c.Data); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

func (d *DB) ListConnectionsByProvider(provider string) ([]model.ProviderConnection, error) {
	rows, err := d.conn.Query(
		`SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt FROM providerConnections WHERE provider = ? AND isActive = 1 ORDER BY priority ASC`,
		provider,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []model.ProviderConnection
	for rows.Next() {
		var c model.ProviderConnection
		var data string
		var name, email sql.NullString
		var priority sql.NullInt64
		var isActive int
		var createdAt, updatedAt string

		if err := rows.Scan(&c.ID, &c.Provider, &c.AuthType, &name, &email, &priority, &isActive, &data, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		c.Name = name.String
		c.Email = email.String
		c.Priority = int(priority.Int64)
		c.IsActive = isActive == 1
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if err := json.Unmarshal([]byte(data), &c.Data); err != nil {
			return nil, err
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

func (d *DB) GetConnection(id string) (*model.ProviderConnection, error) {
	var c model.ProviderConnection
	var data string
	var name, email sql.NullString
	var priority sql.NullInt64
	var isActive int
	var createdAt, updatedAt string

	err := d.conn.QueryRow(
		`SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt FROM providerConnections WHERE id = ?`, id,
	).Scan(&c.ID, &c.Provider, &c.AuthType, &name, &email, &priority, &isActive, &data, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	c.Name = name.String
	c.Email = email.String
	c.Priority = int(priority.Int64)
	c.IsActive = isActive == 1
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if err := json.Unmarshal([]byte(data), &c.Data); err != nil {
		return nil, err
	}
	return &c, nil
}

func (d *DB) CreateConnection(c *model.ProviderConnection) error {
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(c.Data)
	if err != nil {
		return err
	}

	_, err = d.conn.Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Provider, c.AuthType, c.Name, c.Email, c.Priority, boolToInt(c.IsActive), string(data), now, now,
	)
	return err
}

func (d *DB) UpdateConnection(c *model.ProviderConnection) error {
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(c.Data)
	if err != nil {
		return err
	}

	_, err = d.conn.Exec(
		`UPDATE providerConnections SET provider=?, authType=?, name=?, email=?, priority=?, isActive=?, data=?, updatedAt=? WHERE id=?`,
		c.Provider, c.AuthType, c.Name, c.Email, c.Priority, boolToInt(c.IsActive), string(data), now, c.ID,
	)
	return err
}

func (d *DB) DeleteConnection(id string) error {
	_, err := d.conn.Exec(`DELETE FROM providerConnections WHERE id = ?`, id)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
