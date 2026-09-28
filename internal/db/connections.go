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

	conns := []model.ProviderConnection{}
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

	conns := []model.ProviderConnection{}
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

// ErrConnectionCredentialsChanged means a refresh raced with another credential update.
var ErrConnectionCredentialsChanged = errors.New("connection credentials changed")

// UpdateConnectionHealth persists only runtime health, including zero values.
// Never marshal the whole ConnectionData here: it may predate an administrator edit.
func (d *DB) UpdateConnectionHealth(id string, data model.ConnectionData) error {
	return connectionUpdateResult(d.conn.Exec(
		`UPDATE providerConnections SET data=json_set(data,
			'$.testStatus', ?, '$.lastError', ?, '$.rateLimitedUntil', ?, '$.backoffLevel', ?),
			updatedAt=? WHERE id=?`,
		data.TestStatus, data.LastError, data.RateLimitedUntil, data.BackoffLevel,
		time.Now().UTC().Format(time.RFC3339), id,
	))
}

// UpdateConnectionTokens compare-and-swaps credentials, merging only explicitly
// supplied provider keys (not the connection's stale ProviderSpecificData map).
func (d *DB) UpdateConnectionTokens(id, oldAccessToken, oldRefreshToken string, data model.ConnectionData, providerPatch map[string]any) error {
	if providerPatch == nil {
		providerPatch = map[string]any{}
	}
	patch, err := json.Marshal(map[string]any{"providerSpecificData": providerPatch})
	if err != nil {
		return err
	}
	err = connectionUpdateResult(d.conn.Exec(
		`UPDATE providerConnections SET data=json_patch(json_set(data,
			'$.accessToken', ?, '$.refreshToken', ?, '$.expiresAt', ?), ?), updatedAt=?
			WHERE id=? AND COALESCE(json_extract(data, '$.accessToken'), '')=?
			AND COALESCE(json_extract(data, '$.refreshToken'), '')=?`,
		data.AccessToken, data.RefreshToken, data.ExpiresAt, string(patch),
		time.Now().UTC().Format(time.RFC3339), id, oldAccessToken, oldRefreshToken,
	))
	if errors.Is(err, ErrNotFound) {
		var exists int
		if lookupErr := d.conn.QueryRow(`SELECT 1 FROM providerConnections WHERE id=?`, id).Scan(&exists); lookupErr != nil {
			if errors.Is(lookupErr, sql.ErrNoRows) {
				return ErrNotFound
			}
			return lookupErr
		}
		return ErrConnectionCredentialsChanged
	}
	return err
}

// ErrConnectionProfileChanged means enrichment raced with a profile or credential edit.
var ErrConnectionProfileChanged = errors.New("connection profile or credentials changed")

// UpdateConnectionProfile enriches only profile fields, provided the account and
// original profile still match. An empty organizationName leaves provider data alone.
func (d *DB) UpdateConnectionProfile(original *model.ProviderConnection, name, email, organizationName string) error {
	profilePatch := map[string]any{}
	if organizationName != "" {
		profilePatch["providerSpecificData"] = map[string]any{"organizationName": organizationName}
	}
	patch, err := json.Marshal(profilePatch)
	if err != nil {
		return err
	}
	oldOrg, _ := original.Data.ProviderSpecificData["organizationName"].(string)
	oldLegacyOrg, _ := original.Data.ProviderSpecificData["organization_name"].(string)
	err = connectionUpdateResult(d.conn.Exec(
		`UPDATE providerConnections SET name=?, email=?, data=json_patch(data, ?), updatedAt=?
			WHERE id=? AND provider=? AND authType=?
			AND COALESCE(json_extract(data, '$.accessToken'), '')=?
			AND COALESCE(json_extract(data, '$.refreshToken'), '')=?
			AND COALESCE(json_extract(data, '$.apiKey'), '')=?
			AND COALESCE(name, '')=? AND COALESCE(email, '')=?
			AND COALESCE(json_extract(data, '$.providerSpecificData.organizationName'), '')=?
			AND COALESCE(json_extract(data, '$.providerSpecificData.organization_name'), '')=?`,
		name, email, string(patch), time.Now().UTC().Format(time.RFC3339),
		original.ID, original.Provider, original.AuthType, original.Data.AccessToken,
		original.Data.RefreshToken, original.Data.APIKey, original.Name, original.Email, oldOrg, oldLegacyOrg,
	))
	if errors.Is(err, ErrNotFound) {
		var exists int
		if lookupErr := d.conn.QueryRow(`SELECT 1 FROM providerConnections WHERE id=?`, original.ID).Scan(&exists); lookupErr != nil {
			if errors.Is(lookupErr, sql.ErrNoRows) {
				return ErrNotFound
			}
			return lookupErr
		}
		return ErrConnectionProfileChanged
	}
	return err
}

// UpdateConnectionProviderData merges discovered metadata without replacing
// unrelated provider configuration. A nil value explicitly removes a key.
func (d *DB) UpdateConnectionProviderData(id string, providerPatch map[string]any) error {
	if providerPatch == nil {
		providerPatch = map[string]any{}
	}
	patch, err := json.Marshal(map[string]any{"providerSpecificData": providerPatch})
	if err != nil {
		return err
	}
	return connectionUpdateResult(d.conn.Exec(
		`UPDATE providerConnections SET data=json_patch(data, ?), updatedAt=? WHERE id=?`,
		string(patch), time.Now().UTC().Format(time.RFC3339), id,
	))
}

func connectionUpdateResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
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
