package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

const (
	CurrentExportVersion = 1
	ExportGenerator      = "cyrene-gateway"
	RestoreModeReplace   = "replace"
	RestoreModeMerge     = "merge"
)

// ExportPayload represents the versioned backup archive.
type ExportPayload struct {
	Version         int        `json:"version"`
	Generator       string     `json:"generator"`
	ExportedAt      string     `json:"exportedAt"`
	IncludesSecrets bool       `json:"includesSecrets"`
	IncludesUsage   bool       `json:"includesUsage"`
	AuthSecret      string     `json:"authSecret,omitempty"`
	Data            ExportData `json:"data"`
}

type ExportData struct {
	Settings            *Settings                  `json:"settings,omitempty"`
	ProviderConnections []model.ProviderConnection `json:"providerConnections,omitempty"`
	ProviderNodes       []model.ProviderNode       `json:"providerNodes,omitempty"`
	ProxyPools          []model.ProxyPool          `json:"proxyPools,omitempty"`
	APIKeys             []model.APIKey             `json:"apiKeys,omitempty"`
	Combos              []model.Combo              `json:"combos,omitempty"`
	KV                  []KVEntry                  `json:"kv,omitempty"`
	UsageHistory        []UsageEntry               `json:"usageHistory,omitempty"`
	UsageDaily          []DailyUsageEntry          `json:"usageDaily,omitempty"`
	RequestDetails      []RequestDetail            `json:"requestDetails,omitempty"`
}

type KVEntry struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type DailyUsageEntry struct {
	DateKey string `json:"dateKey"`
	Data    string `json:"data"`
}

// ExportData builds a versioned snapshot of the database.
func (d *DB) ExportData(includeSecrets, includeUsage bool) (*ExportPayload, error) {
	data := ExportData{}

	// 1. Settings
	settings, err := d.GetSettings()
	if err != nil && err != sql.ErrNoRows && err != ErrNotFound {
		return nil, fmt.Errorf("export settings: %w", err)
	}
	if settings != nil {
		sCopy := *settings
		if !includeSecrets {
			sCopy.PasswordHash = ""
		}
		data.Settings = &sCopy
	}

	// 2. Provider Connections
	conns, err := d.ListConnections()
	if err != nil {
		return nil, fmt.Errorf("export providerConnections: %w", err)
	}
	connsCopy := make([]model.ProviderConnection, len(conns))
	for i, c := range conns {
		if !includeSecrets {
			c.Data.APIKey = ""
			c.Data.AccessToken = ""
			c.Data.RefreshToken = ""
			c.Data.ProviderSpecificData = nil
		}
		connsCopy[i] = c
	}
	data.ProviderConnections = connsCopy

	// 3. Provider Nodes
	nodes, err := d.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("export providerNodes: %w", err)
	}
	nodesCopy := make([]model.ProviderNode, len(nodes))
	for i, n := range nodes {
		if !includeSecrets {
			n.Data.APIKey = ""
		}
		nodesCopy[i] = n
	}
	data.ProviderNodes = nodesCopy

	// 4. Proxy Pools
	pools, err := d.ListProxyPools()
	if err != nil {
		return nil, fmt.Errorf("export proxyPools: %w", err)
	}
	data.ProxyPools = pools

	// 5. API Keys
	keys, err := d.ListAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("export apiKeys: %w", err)
	}
	keysCopy := make([]model.APIKey, len(keys))
	for i, k := range keys {
		if !includeSecrets {
			k.Key = ""
		}
		keysCopy[i] = k
	}
	data.APIKeys = keysCopy

	// 6. Combos
	combos, err := d.ListCombos()
	if err != nil {
		return nil, fmt.Errorf("export combos: %w", err)
	}
	data.Combos = combos

	// 7. KV entries
	kvRows, err := d.conn.Query(`SELECT scope, key, value FROM kv ORDER BY scope, key`)
	if err != nil {
		return nil, fmt.Errorf("export kv: %w", err)
	}
	defer kvRows.Close()
	var kvList []KVEntry
	for kvRows.Next() {
		var entry KVEntry
		if err := kvRows.Scan(&entry.Scope, &entry.Key, &entry.Value); err != nil {
			return nil, fmt.Errorf("scan kv: %w", err)
		}
		kvList = append(kvList, entry)
	}
	data.KV = kvList

	// 8. Optional Usage
	if includeUsage {
		usageEntries, err := d.GetUsageHistory(UsageFilter{Limit: 100000})
		if err == nil {
			data.UsageHistory = usageEntries
		}

		dailyRows, err := d.conn.Query(`SELECT dateKey, data FROM usageDaily ORDER BY dateKey DESC`)
		if err == nil {
			defer dailyRows.Close()
			var dailyList []DailyUsageEntry
			for dailyRows.Next() {
				var entry DailyUsageEntry
				if err := dailyRows.Scan(&entry.DateKey, &entry.Data); err == nil {
					dailyList = append(dailyList, entry)
				}
			}
			data.UsageDaily = dailyList
		}

		reqRows, err := d.conn.Query(`SELECT id, timestamp, provider, model, connectionId, status, data FROM requestDetails ORDER BY timestamp DESC LIMIT 10000`)
		if err == nil {
			defer reqRows.Close()
			var reqList []RequestDetail
			for reqRows.Next() {
				var rd RequestDetail
				var prov, mod, connID, st sql.NullString
				if err := reqRows.Scan(&rd.ID, &rd.Timestamp, &prov, &mod, &connID, &st, &rd.Data); err == nil {
					rd.Provider = prov.String
					rd.Model = mod.String
					rd.ConnectionID = connID.String
					rd.Status = st.String
					reqList = append(reqList, rd)
				}
			}
			data.RequestDetails = reqList
		}
	}

	payload := &ExportPayload{
		Version:         CurrentExportVersion,
		Generator:       ExportGenerator,
		ExportedAt:      time.Now().UTC().Format(time.RFC3339),
		IncludesSecrets: includeSecrets,
		IncludesUsage:   includeUsage,
		Data:            data,
	}

	return payload, nil
}

// ImportData transactionally imports the backup payload into the database.
// Mode can be RestoreModeReplace (default full overwrite) or RestoreModeMerge (upsert).
func (d *DB) ImportData(payload *ExportPayload, mode string) error {
	if payload == nil {
		return fmt.Errorf("empty backup payload")
	}
	if payload.Version <= 0 || payload.Version > CurrentExportVersion {
		return fmt.Errorf("unsupported backup schema version %d (supported up to %d)", payload.Version, CurrentExportVersion)
	}

	tx, err := d.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// 1. Snapshot existing credentials to prevent sanitised backup imports from wiping real tokens
	existingConns := make(map[string]model.ProviderConnection)
	cRows, err := tx.Query(`SELECT id, data FROM providerConnections`)
	if err == nil {
		for cRows.Next() {
			var id, rawData string
			if err := cRows.Scan(&id, &rawData); err == nil {
				var pc model.ProviderConnection
				pc.ID = id
				_ = json.Unmarshal([]byte(rawData), &pc.Data)
				existingConns[id] = pc
			}
		}
		cRows.Close()
	}

	existingNodes := make(map[string]model.ProviderNode)
	nRows, err := tx.Query(`SELECT id, data FROM providerNodes`)
	if err == nil {
		for nRows.Next() {
			var id, rawData string
			if err := nRows.Scan(&id, &rawData); err == nil {
				var pn model.ProviderNode
				pn.ID = id
				_ = json.Unmarshal([]byte(rawData), &pn.Data)
				existingNodes[id] = pn
			}
		}
		nRows.Close()
	}

	existingKeys := make(map[string]string)
	kRows, err := tx.Query(`SELECT id, key FROM apiKeys`)
	if err == nil {
		for kRows.Next() {
			var id, key string
			if err := kRows.Scan(&id, &key); err == nil {
				existingKeys[id] = key
			}
		}
		kRows.Close()
	}

	var existingPasswordHash string
	_ = tx.QueryRow(`SELECT json_extract(data, '$.passwordHash') FROM settings WHERE id = 1`).Scan(&existingPasswordHash)

	// If replace mode, clear non-system user tables
	if mode == RestoreModeReplace {
		tables := []string{
			"providerConnections", "providerNodes", "proxyPools", "apiKeys", "combos", "kv",
		}
		if payload.IncludesUsage {
			tables = append(tables, "usageHistory", "usageDaily", "requestDetails")
		}
		for _, tbl := range tables {
			if _, err := tx.Exec(`DELETE FROM ` + tbl); err != nil {
				return fmt.Errorf("clear table %s: %w", tbl, err)
			}
		}
	}

	// 2. Restore Settings
	if payload.Data.Settings != nil {
		st := *payload.Data.Settings
		// If imported hash is empty but existing was set, preserve existing hash
		if st.PasswordHash == "" && existingPasswordHash != "" {
			st.PasswordHash = existingPasswordHash
		}
		dataBytes, err := json.Marshal(st)
		if err != nil {
			return fmt.Errorf("marshal settings: %w", err)
		}
		_, err = tx.Exec(
			`INSERT INTO settings (id, data) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
			string(dataBytes),
		)
		if err != nil {
			return fmt.Errorf("restore settings: %w", err)
		}
	}

	// 3. Restore Provider Connections
	stmtConn, err := tx.Prepare(`
		INSERT INTO providerConnections (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider = excluded.provider,
			authType = excluded.authType,
			name = excluded.name,
			email = excluded.email,
			priority = excluded.priority,
			isActive = excluded.isActive,
			data = excluded.data,
			updatedAt = excluded.updatedAt
	`)
	if err != nil {
		return fmt.Errorf("prepare connection upsert: %w", err)
	}
	defer stmtConn.Close()

	for _, c := range payload.Data.ProviderConnections {
		if c.ID == "" {
			continue
		}
		// Credential preservation for sanitized import
		if existing, ok := existingConns[c.ID]; ok {
			if c.Data.APIKey == "" && existing.Data.APIKey != "" {
				c.Data.APIKey = existing.Data.APIKey
			}
			if c.Data.AccessToken == "" && existing.Data.AccessToken != "" {
				c.Data.AccessToken = existing.Data.AccessToken
			}
			if c.Data.RefreshToken == "" && existing.Data.RefreshToken != "" {
				c.Data.RefreshToken = existing.Data.RefreshToken
			}
			if c.Data.ProviderSpecificData == nil && existing.Data.ProviderSpecificData != nil {
				c.Data.ProviderSpecificData = existing.Data.ProviderSpecificData
			}
		}
		dataBytes, err := json.Marshal(c.Data)
		if err != nil {
			return fmt.Errorf("marshal connection %s: %w", c.ID, err)
		}
		created := c.CreatedAt.UTC().Format(time.RFC3339)
		if c.CreatedAt.IsZero() {
			created = time.Now().UTC().Format(time.RFC3339)
		}
		updated := time.Now().UTC().Format(time.RFC3339)
		activeInt := 0
		if c.IsActive {
			activeInt = 1
		}
		if _, err := stmtConn.Exec(c.ID, c.Provider, c.AuthType, c.Name, c.Email, c.Priority, activeInt, string(dataBytes), created, updated); err != nil {
			return fmt.Errorf("upsert connection %s: %w", c.ID, err)
		}
	}

	// 4. Restore Provider Nodes
	stmtNode, err := tx.Prepare(`
		INSERT INTO providerNodes (id, type, name, data, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			name = excluded.name,
			data = excluded.data,
			updatedAt = excluded.updatedAt
	`)
	if err != nil {
		return fmt.Errorf("prepare node upsert: %w", err)
	}
	defer stmtNode.Close()

	for _, n := range payload.Data.ProviderNodes {
		if n.ID == "" {
			continue
		}
		if existing, ok := existingNodes[n.ID]; ok {
			if n.Data.APIKey == "" && existing.Data.APIKey != "" {
				n.Data.APIKey = existing.Data.APIKey
			}
		}
		dataBytes, err := json.Marshal(n.Data)
		if err != nil {
			return fmt.Errorf("marshal node %s: %w", n.ID, err)
		}
		created := n.CreatedAt.UTC().Format(time.RFC3339)
		if n.CreatedAt.IsZero() {
			created = time.Now().UTC().Format(time.RFC3339)
		}
		updated := time.Now().UTC().Format(time.RFC3339)
		if _, err := stmtNode.Exec(n.ID, n.Type, n.Name, string(dataBytes), created, updated); err != nil {
			return fmt.Errorf("upsert node %s: %w", n.ID, err)
		}
	}

	// 5. Restore Proxy Pools
	stmtPool, err := tx.Prepare(`
		INSERT INTO proxyPools (id, isActive, testStatus, data, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			isActive = excluded.isActive,
			testStatus = excluded.testStatus,
			data = excluded.data,
			updatedAt = excluded.updatedAt
	`)
	if err != nil {
		return fmt.Errorf("prepare proxyPool upsert: %w", err)
	}
	defer stmtPool.Close()

	for _, p := range payload.Data.ProxyPools {
		if p.ID == "" {
			continue
		}
		dataBytes, err := json.Marshal(p.Data)
		if err != nil {
			return fmt.Errorf("marshal proxy pool %s: %w", p.ID, err)
		}
		created := p.CreatedAt.UTC().Format(time.RFC3339)
		if p.CreatedAt.IsZero() {
			created = time.Now().UTC().Format(time.RFC3339)
		}
		updated := time.Now().UTC().Format(time.RFC3339)
		activeInt := 0
		if p.IsActive {
			activeInt = 1
		}
		if _, err := stmtPool.Exec(p.ID, activeInt, p.TestStatus, string(dataBytes), created, updated); err != nil {
			return fmt.Errorf("upsert proxy pool %s: %w", p.ID, err)
		}
	}

	// 6. Restore API Keys
	stmtKey, err := tx.Prepare(`
		INSERT INTO apiKeys (id, key, name, machineId, isActive, allowedModels, rpm, systemPrompt, expiresAt, createdAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			key = excluded.key,
			name = excluded.name,
			machineId = excluded.machineId,
			isActive = excluded.isActive,
			allowedModels = excluded.allowedModels,
			rpm = excluded.rpm,
			systemPrompt = excluded.systemPrompt,
			expiresAt = excluded.expiresAt
	`)
	if err != nil {
		return fmt.Errorf("prepare apiKey upsert: %w", err)
	}
	defer stmtKey.Close()

	for _, k := range payload.Data.APIKeys {
		if k.ID == "" {
			continue
		}
		// If key string is omitted/empty and existing key was present, preserve existing key
		if k.Key == "" {
			if existingKey, ok := existingKeys[k.ID]; ok && existingKey != "" {
				k.Key = existingKey
			} else {
				// Don't insert an invalid key with empty string
				continue
			}
		}
		var allowedModelsJSON string
		if len(k.AllowedModels) > 0 {
			if b, err := json.Marshal(k.AllowedModels); err == nil {
				allowedModelsJSON = string(b)
			}
		}
		activeInt := 0
		if k.IsActive {
			activeInt = 1
		}
		created := k.CreatedAt.UTC().Format(time.RFC3339)
		if k.CreatedAt.IsZero() {
			created = time.Now().UTC().Format(time.RFC3339)
		}
		if _, err := stmtKey.Exec(k.ID, k.Key, k.Name, k.MachineID, activeInt, allowedModelsJSON, k.RPM, k.SystemPrompt, k.ExpiresAt, created); err != nil {
			return fmt.Errorf("upsert apiKey %s: %w", k.ID, err)
		}
	}
	// 7. Restore Combos
	stmtCombo, err := tx.Prepare(`
		INSERT INTO combos (id, name, kind, models, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			models = excluded.models,
			updatedAt = excluded.updatedAt
	`)
	if err != nil {
		return fmt.Errorf("prepare combo upsert: %w", err)
	}
	defer stmtCombo.Close()

	for _, c := range payload.Data.Combos {
		if c.ID == "" || c.Name == "" {
			continue
		}
		modelsBytes, err := json.Marshal(c.Models)
		if err != nil {
			return fmt.Errorf("marshal combo %s: %w", c.ID, err)
		}
		created := c.CreatedAt.UTC().Format(time.RFC3339)
		if c.CreatedAt.IsZero() {
			created = time.Now().UTC().Format(time.RFC3339)
		}
		updated := time.Now().UTC().Format(time.RFC3339)
		if _, err := stmtCombo.Exec(c.ID, c.Name, c.Kind, string(modelsBytes), created, updated); err != nil {
			return fmt.Errorf("upsert combo %s: %w", c.ID, err)
		}
	}

	// 8. Restore KV
	stmtKV, err := tx.Prepare(`
		INSERT INTO kv (scope, key, value)
		VALUES (?, ?, ?)
		ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value
	`)
	if err != nil {
		return fmt.Errorf("prepare kv upsert: %w", err)
	}
	defer stmtKV.Close()

	for _, kv := range payload.Data.KV {
		if kv.Scope == "" || kv.Key == "" {
			continue
		}
		if _, err := stmtKV.Exec(kv.Scope, kv.Key, kv.Value); err != nil {
			return fmt.Errorf("upsert kv (%s, %s): %w", kv.Scope, kv.Key, err)
		}
	}

	// 9. Restore Usage if included
	if payload.IncludesUsage {
		if len(payload.Data.UsageHistory) > 0 {
			stmtUsage, err := tx.Prepare(`
				INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`)
			if err != nil {
				return fmt.Errorf("prepare usage insert: %w", err)
			}
			defer stmtUsage.Close()
			for _, u := range payload.Data.UsageHistory {
				if _, err := stmtUsage.Exec(u.Timestamp, u.Provider, u.Model, u.ConnectionID, u.APIKey, u.Endpoint, u.PromptTokens, u.CompletionTokens, u.Cost, u.Status, u.Tokens, u.Meta); err != nil {
					return fmt.Errorf("insert usage entry: %w", err)
				}
			}
		}

		if len(payload.Data.UsageDaily) > 0 {
			stmtDaily, err := tx.Prepare(`
				INSERT INTO usageDaily (dateKey, data) VALUES (?, ?)
				ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data
			`)
			if err != nil {
				return fmt.Errorf("prepare usageDaily insert: %w", err)
			}
			defer stmtDaily.Close()
			for _, dEntry := range payload.Data.UsageDaily {
				if _, err := stmtDaily.Exec(dEntry.DateKey, dEntry.Data); err != nil {
					return fmt.Errorf("insert usageDaily entry: %w", err)
				}
			}
		}

		if len(payload.Data.RequestDetails) > 0 {
			stmtReq, err := tx.Prepare(`
				INSERT INTO requestDetails (id, timestamp, provider, model, connectionId, status, data)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					timestamp = excluded.timestamp,
					provider = excluded.provider,
					model = excluded.model,
					connectionId = excluded.connectionId,
					status = excluded.status,
					data = excluded.data
			`)
			if err != nil {
				return fmt.Errorf("prepare requestDetails insert: %w", err)
			}
			defer stmtReq.Close()
			for _, rd := range payload.Data.RequestDetails {
				if _, err := stmtReq.Exec(rd.ID, rd.Timestamp, rd.Provider, rd.Model, rd.ConnectionID, rd.Status, rd.Data); err != nil {
					return fmt.Errorf("insert requestDetail entry: %w", err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore transaction: %w", err)
	}
	tx = nil // disarm rollback in defer

	return nil
}
