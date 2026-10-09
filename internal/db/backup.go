package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
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

func redactProxyURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		if !strings.Contains(raw, "@") {
			return raw
		}
		return ""
	}
	u.User = nil
	return u.String()
}

var secretKVMarkers = []string{"secret", "token", "password", "credential", "api_key", "apikey", "private_key"}

func isSecretKVKey(scope, key string) bool {
	name := strings.ToLower(scope + ":" + key)
	for _, marker := range secretKVMarkers {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
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
	poolsCopy := make([]model.ProxyPool, len(pools))
	for i, p := range pools {
		if !includeSecrets {
			p.Data.ProxyURL = redactProxyURL(p.Data.ProxyURL)
		}
		poolsCopy[i] = p
	}
	data.ProxyPools = poolsCopy

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
		if !includeSecrets && isSecretKVKey(entry.Scope, entry.Key) {
			entry.Value = ""
		}
		kvList = append(kvList, entry)
	}
	data.KV = kvList

	// 8. Optional Usage
	if includeUsage {
		usageEntries, err := d.GetUsageHistory(UsageFilter{Limit: 100000})
		if err == nil {
			if !includeSecrets {
				for i := range usageEntries {
					usageEntries[i].APIKey = ""
					usageEntries[i].Meta = ""
				}
			}
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
					if !includeSecrets {
						rd.Data = ""
					}
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

	// Plan usage before any writes. A shallow copy keeps the caller's backup reusable.
	usage := payload.Data
	var usageLifetime int64
	if payload.IncludesUsage {
		usage, usageLifetime, err = prepareUsageImport(tx, usage, mode == RestoreModeReplace)
		if err != nil {
			return err
		}
	}

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
	existingPools := make(map[string]model.ProxyPool)
	pRows, err := tx.Query(`SELECT id, data FROM proxyPools`)
	if err == nil {
		for pRows.Next() {
			var id, rawData string
			if err := pRows.Scan(&id, &rawData); err == nil {
				var pp model.ProxyPool
				pp.ID = id
				_ = json.Unmarshal([]byte(rawData), &pp.Data)
				existingPools[id] = pp
			}
		}
		pRows.Close()
	}

	existingKV := make(map[string]string)
	kvExistingRows, err := tx.Query(`SELECT scope, key, value FROM kv`)
	if err == nil {
		for kvExistingRows.Next() {
			var scope, key, val string
			if err := kvExistingRows.Scan(&scope, &key, &val); err == nil {
				existingKV[scope+"\x00"+key] = val
			}
		}
		kvExistingRows.Close()
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
		if existing, ok := existingPools[p.ID]; ok {
			if (p.Data.ProxyURL == "" || p.Data.ProxyURL == redactProxyURL(existing.Data.ProxyURL)) && existing.Data.ProxyURL != "" {
				p.Data.ProxyURL = existing.Data.ProxyURL
			}
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
		if kv.Value == "" && isSecretKVKey(kv.Scope, kv.Key) {
			if existingVal, ok := existingKV[kv.Scope+"\x00"+kv.Key]; ok && existingVal != "" {
				kv.Value = existingVal
			}
		}
		if _, err := stmtKV.Exec(kv.Scope, kv.Key, kv.Value); err != nil {
			return fmt.Errorf("upsert kv (%s, %s): %w", kv.Scope, kv.Key, err)
		}
	}

	// 9. Restore Usage if included
	if payload.IncludesUsage {
		if len(usage.UsageHistory) > 0 {
			stmtUsage, err := tx.Prepare(`
				INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`)
			if err != nil {
				return fmt.Errorf("prepare usage insert: %w", err)
			}
			defer stmtUsage.Close()
			for _, u := range usage.UsageHistory {
				if _, err := stmtUsage.Exec(u.Timestamp, u.Provider, u.Model, u.ConnectionID, u.APIKey, u.Endpoint, u.PromptTokens, u.CompletionTokens, u.Cost, u.Status, u.Tokens, u.Meta); err != nil {
					return fmt.Errorf("insert usage entry: %w", err)
				}
			}
		}

		if len(usage.UsageDaily) > 0 {
			stmtDaily, err := tx.Prepare(`
				INSERT INTO usageDaily (dateKey, data) VALUES (?, ?)
				ON CONFLICT(dateKey) DO UPDATE SET data = excluded.data
			`)
			if err != nil {
				return fmt.Errorf("prepare usageDaily insert: %w", err)
			}
			defer stmtDaily.Close()
			for _, dEntry := range usage.UsageDaily {
				if _, err := stmtDaily.Exec(dEntry.DateKey, dEntry.Data); err != nil {
					return fmt.Errorf("insert usageDaily entry: %w", err)
				}
			}
		}

		if _, err := tx.Exec(`INSERT INTO _meta (key, value) VALUES ('totalRequestsLifetime', ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprint(usageLifetime)); err != nil {
			return fmt.Errorf("restore usage lifetime: %w", err)
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
			if _, err := tx.Exec(
				`DELETE FROM requestDetails WHERE id IN (
				   SELECT id FROM requestDetails ORDER BY timestamp DESC, id DESC LIMIT -1 OFFSET ?
				 )`, defaultMaxRequestDetails,
			); err != nil {
				return fmt.Errorf("trim requestDetails after restore: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore transaction: %w", err)
	}
	tx = nil // disarm rollback in defer

	return nil
}

// prepareUsageImport uses a multiset union of all persisted event fields, never IDs:
// IDs are database-local, and equal legitimate events can occur more than once.
// Aggregate-only overlaps have no provable identity; reject rather than guess.
func prepareUsageImport(tx *sql.Tx, data ExportData, replace bool) (ExportData, int64, error) {
	incomingHistory, err := usageDaysFromHistory(data.UsageHistory)
	if err != nil {
		return data, 0, err
	}
	if replace {
		// Keep replace's verbatim rows, including legacy undercounted daily blobs.
		// Lifetime includes pruned daily requests without dropping retained history.
		for _, entry := range data.UsageDaily {
			var day DayData
			if err := json.Unmarshal([]byte(entry.Data), &day); err != nil {
				return data, 0, fmt.Errorf("invalid usage daily data for %s: %w", entry.DateKey, err)
			}
			day.Requests = max(day.Requests, incomingHistory[entry.DateKey].Requests)
			incomingHistory[entry.DateKey] = day
		}
		var lifetime int64
		for _, day := range incomingHistory {
			lifetime += int64(day.Requests)
		}
		return data, lifetime, nil
	}
	incomingDays, err := usageDaysForImport(data.UsageDaily, incomingHistory)
	if err != nil {
		return data, 0, err
	}

	rows, err := tx.Query(`SELECT ` + usageHistoryColumns + ` FROM usageHistory`)
	if err != nil {
		return data, 0, fmt.Errorf("read existing usage: %w", err)
	}
	localEntries, err := scanUsageHistory(rows)
	if err != nil {
		return data, 0, err
	}
	localHistory, err := usageDaysFromHistory(localEntries)
	if err != nil {
		return data, 0, err
	}
	rows, err = tx.Query(`SELECT dateKey, data FROM usageDaily`)
	if err != nil {
		return data, 0, err
	}
	var localDaily []DailyUsageEntry
	for rows.Next() {
		var day DailyUsageEntry
		if err := rows.Scan(&day.DateKey, &day.Data); err != nil {
			rows.Close()
			return data, 0, err
		}
		localDaily = append(localDaily, day)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return data, 0, err
	}
	localDays, err := usageDaysForImport(localDaily, localHistory)
	if err != nil {
		return data, 0, err
	}

	counts := make(map[UsageEntry]int, len(localEntries))
	for _, e := range localEntries {
		e.ID = 0
		counts[e]++
	}
	var additions []UsageEntry
	addedByDay := make(map[string][]UsageEntry)
	for _, e := range data.UsageHistory {
		e.ID = 0
		if counts[e] > 0 {
			counts[e]--
			continue
		}
		additions = append(additions, e)
		date := dateKeyFromTimestamp(e.Timestamp)
		addedByDay[date] = append(addedByDay[date], e)
	}

	var lifetime, recordedLifetime int64
	for _, day := range localDays {
		lifetime += int64(day.Requests)
	}
	err = tx.QueryRow(`SELECT CAST(value AS INTEGER) FROM _meta WHERE key = 'totalRequestsLifetime'`).Scan(&recordedLifetime)
	if err != nil && err != sql.ErrNoRows {
		return data, 0, err
	}
	// A previously pruned lifetime must never be reset to the remaining row count.
	lifetime = max(lifetime, recordedLifetime)
	var mergedDaily []DailyUsageEntry
	for date, incoming := range incomingDays {
		local := localDays[date]
		merged := incoming
		if local.Requests > 0 {
			incomingComplete := usageDayCovers(incomingHistory[date], incoming)
			localComplete := usageDayCovers(localHistory[date], local)
			if !incomingComplete || (!localComplete && len(addedByDay[date]) > 0) {
				return data, 0, fmt.Errorf("cannot merge usage for %s: overlapping aggregate-only data has missing history; use replace mode", date)
			}
			merged = local
			for _, e := range addedByDay[date] {
				addUsageToDay(&merged, e)
			}
		}
		lifetime += int64(merged.Requests - local.Requests)
		raw, err := json.Marshal(merged)
		if err != nil {
			return data, 0, err
		}
		mergedDaily = append(mergedDaily, DailyUsageEntry{DateKey: date, Data: string(raw)})
	}
	data.UsageHistory = additions
	data.UsageDaily = mergedDaily
	return data, lifetime, nil
}

func usageDaysFromHistory(entries []UsageEntry) (map[string]DayData, error) {
	days := make(map[string]DayData)
	for _, e := range entries {
		if _, err := time.Parse(time.RFC3339, e.Timestamp); err != nil {
			return nil, fmt.Errorf("invalid usage timestamp %q; use replace with a valid backup: %w", e.Timestamp, err)
		}
		date := dateKeyFromTimestamp(e.Timestamp)
		day := days[date]
		addUsageToDay(&day, e)
		days[date] = day
	}
	return days, nil
}

// Missing daily rows can be reconstructed from history. Existing aggregates must
// cover every retained history dimension; malformed or contradictory data fails closed.
func usageDaysForImport(entries []DailyUsageEntry, history map[string]DayData) (map[string]DayData, error) {
	days := make(map[string]DayData, len(history)+len(entries))
	for date, day := range history {
		days[date] = day
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		var day DayData
		_, dateErr := time.Parse("2006-01-02", entry.DateKey)
		if err := json.Unmarshal([]byte(entry.Data), &day); err != nil || dateErr != nil || seen[entry.DateKey] {
			return nil, fmt.Errorf("invalid or duplicate usage daily data for %s; use replace with a valid backup", entry.DateKey)
		}
		seen[entry.DateKey] = true
		if !usageDayCovers(day, history[entry.DateKey]) || !validUsageDay(day) {
			return nil, fmt.Errorf("inconsistent usage daily data for %s; use replace with a valid backup", entry.DateKey)
		}
		days[entry.DateKey] = day
	}
	return days, nil
}

func usageCounterCovers(a, b DayCounter) bool {
	// Summation order changes during export/merge; allow only floating-point noise.
	epsilon := 1e-12 * max(1, math.Abs(a.Cost), math.Abs(b.Cost))
	return a.Requests >= b.Requests && a.PromptTokens >= b.PromptTokens &&
		a.CompletionTokens >= b.CompletionTokens && a.Cost+epsilon >= b.Cost
}

func usageDayCovers(a, b DayData) bool {
	if !usageCounterCovers(DayCounter{a.Requests, a.PromptTokens, a.CompletionTokens, a.Cost},
		DayCounter{b.Requests, b.PromptTokens, b.CompletionTokens, b.Cost}) {
		return false
	}
	for key, counter := range b.ByProvider {
		if !usageCounterCovers(a.ByProvider[key], counter) {
			return false
		}
	}
	for key, counter := range b.ByModel {
		if !usageCounterCovers(a.ByModel[key], counter) {
			return false
		}
	}
	return true
}

func validUsageDay(day DayData) bool {
	total := DayCounter{day.Requests, day.PromptTokens, day.CompletionTokens, day.Cost}
	if !usageCounterCovers(total, DayCounter{}) {
		return false
	}
	for _, dimension := range []map[string]DayCounter{day.ByProvider, day.ByModel} {
		var sum DayCounter
		for _, c := range dimension {
			if !usageCounterCovers(c, DayCounter{}) {
				return false
			}
			sum.Requests += c.Requests
			sum.PromptTokens += c.PromptTokens
			sum.CompletionTokens += c.CompletionTokens
			sum.Cost += c.Cost
		}
		if !usageCounterCovers(total, sum) {
			return false
		}
	}
	return true
}
