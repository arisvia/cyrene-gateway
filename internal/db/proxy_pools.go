package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Proxy Pools repository (outbound proxy rotation)

func (d *DB) ListProxyPools() ([]model.ProxyPool, error) {
	rows, err := d.conn.Query(
		`SELECT id, isActive, testStatus, data, createdAt, updatedAt FROM proxyPools ORDER BY createdAt ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pools []model.ProxyPool
	for rows.Next() {
		var p model.ProxyPool
		var data string
		var testStatus sql.NullString
		var isActive int
		var createdAt, updatedAt string

		if err := rows.Scan(&p.ID, &isActive, &testStatus, &data, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		p.IsActive = isActive == 1
		p.TestStatus = testStatus.String
		p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if err := json.Unmarshal([]byte(data), &p.Data); err != nil {
			return nil, err
		}
		pools = append(pools, p)
	}
	return pools, rows.Err()
}

func (d *DB) GetProxyPool(id string) (*model.ProxyPool, error) {
	var p model.ProxyPool
	var data string
	var testStatus sql.NullString
	var isActive int
	var createdAt, updatedAt string

	err := d.conn.QueryRow(
		`SELECT id, isActive, testStatus, data, createdAt, updatedAt FROM proxyPools WHERE id = ?`, id,
	).Scan(&p.ID, &isActive, &testStatus, &data, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	p.IsActive = isActive == 1
	p.TestStatus = testStatus.String
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if err := json.Unmarshal([]byte(data), &p.Data); err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) CreateProxyPool(p *model.ProxyPool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(p.Data)
	if err != nil {
		return err
	}

	isActive := 0
	if p.IsActive {
		isActive = 1
	}

	_, err = d.conn.Exec(
		`INSERT INTO proxyPools (id, isActive, testStatus, data, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, isActive, p.TestStatus, string(data), now, now,
	)
	return err
}

func (d *DB) UpdateProxyPool(p *model.ProxyPool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(p.Data)
	if err != nil {
		return err
	}

	isActive := 0
	if p.IsActive {
		isActive = 1
	}

	_, err = d.conn.Exec(
		`UPDATE proxyPools SET isActive=?, testStatus=?, data=?, updatedAt=? WHERE id=?`,
		isActive, p.TestStatus, string(data), now, p.ID,
	)
	return err
}

func (d *DB) DeleteProxyPool(id string) error {
	_, err := d.conn.Exec(`DELETE FROM proxyPools WHERE id = ?`, id)
	return err
}
