package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const defaultMaxRequestDetails = 200

// RequestDetail represents a stored request detail record.
type RequestDetail struct {
	ID           string `json:"id"`
	Timestamp    string `json:"timestamp"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
	Status       string `json:"status,omitempty"`
	Data         string `json:"data"`
}

// RequestDetailFilter for querying request details.
type RequestDetailFilter struct {
	Provider     string
	Model        string
	ConnectionID string
	Status       string
	StartDate    string
	EndDate      string
	Page         int
	PageSize     int
}

// RequestDetailResult is the paginated response.
type RequestDetailResult struct {
	Details    []json.RawMessage `json:"details"`
	Pagination Pagination        `json:"pagination"`
}

// Pagination metadata.
type Pagination struct {
	Page       int  `json:"page"`
	PageSize   int  `json:"pageSize"`
	TotalItems int  `json:"totalItems"`
	TotalPages int  `json:"totalPages"`
	HasNext    bool `json:"hasNext"`
	HasPrev    bool `json:"hasPrev"`
}

// SaveRequestDetail inserts or updates a request detail record and trims old records.
func (d *DB) SaveRequestDetail(rd *RequestDetail) error {
	if rd.ID == "" {
		rd.ID = fmt.Sprintf("%d-%s", time.Now().UnixNano(), rd.Model)
	}
	if rd.Timestamp == "" {
		rd.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	_, err := d.conn.Exec(
		`INSERT INTO requestDetails (id, timestamp, provider, model, connectionId, status, data)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET timestamp=excluded.timestamp, provider=excluded.provider,
		   model=excluded.model, connectionId=excluded.connectionId, status=excluded.status, data=excluded.data`,
		rd.ID, rd.Timestamp, rd.Provider, rd.Model, rd.ConnectionID, rd.Status, rd.Data,
	)
	if err != nil {
		return fmt.Errorf("save request detail: %w", err)
	}

	// Trim old records beyond max
	var count int
	if err := d.conn.QueryRow(`SELECT COUNT(*) FROM requestDetails`).Scan(&count); err == nil {
		if count > defaultMaxRequestDetails {
			d.conn.Exec(
				`DELETE FROM requestDetails WHERE id IN (SELECT id FROM requestDetails ORDER BY timestamp ASC LIMIT ?)`,
				count-defaultMaxRequestDetails,
			)
		}
	}

	return nil
}

// GetRequestDetails returns paginated request details with optional filters.
func (d *DB) GetRequestDetails(f RequestDetailFilter) (*RequestDetailResult, error) {
	var conds []string
	var args []any

	if f.Provider != "" {
		conds = append(conds, "provider = ?")
		args = append(args, f.Provider)
	}
	if f.Model != "" {
		conds = append(conds, "model = ?")
		args = append(args, f.Model)
	}
	if f.ConnectionID != "" {
		conds = append(conds, "connectionId = ?")
		args = append(args, f.ConnectionID)
	}
	if f.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, f.Status)
	}
	if f.StartDate != "" {
		conds = append(conds, "timestamp >= ?")
		args = append(args, f.StartDate)
	}
	if f.EndDate != "" {
		conds = append(conds, "timestamp <= ?")
		args = append(args, f.EndDate)
	}

	where := ""
	if len(conds) > 0 {
		for i, c := range conds {
			if i == 0 {
				where += " WHERE " + c
			} else {
				where += " AND " + c
			}
		}
	}

	// Count total
	var totalItems int
	countQuery := `SELECT COUNT(*) FROM requestDetails` + where
	if err := d.conn.QueryRow(countQuery, args...).Scan(&totalItems); err != nil {
		return nil, err
	}

	page := f.Page
	if page < 1 {
		page = 1
	}
	pageSize := f.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	totalPages := (totalItems + pageSize - 1) / pageSize
	offset := (page - 1) * pageSize

	// Fetch page
	dataQuery := `SELECT data FROM requestDetails` + where + ` ORDER BY timestamp DESC LIMIT ? OFFSET ?`
	dataArgs := append(args, pageSize, offset)
	rows, err := d.conn.Query(dataQuery, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var details []json.RawMessage
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		details = append(details, json.RawMessage(data))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if details == nil {
		details = []json.RawMessage{}
	}

	return &RequestDetailResult{
		Details: details,
		Pagination: Pagination{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: totalItems,
			TotalPages: totalPages,
			HasNext:    page < totalPages,
			HasPrev:    page > 1,
		},
	}, nil
}

// GetRequestDetailByID returns a single request detail by ID.
func (d *DB) GetRequestDetailByID(id string) (json.RawMessage, error) {
	var data string
	err := d.conn.QueryRow(`SELECT data FROM requestDetails WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
