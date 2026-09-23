package model

// models.dev catalog backfill (Phase 36 T6): when a provider's own models API
// omits context_length / max_output metadata, backfill from
// https://models.dev/api.json (9router capabilities.js uses the same source).
// The catalog is cached in memory and matched by fuzzy id (exact → lowercase
// exact → id containment).

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ModelsDevURL is the catalog source; var so tests can point it at a mock.
var ModelsDevURL = "https://models.dev/api.json"

const modelsDevCacheTTL = 24 * time.Hour

// ModelsDevEntry is the subset of a models.dev model record we need.
type ModelsDevEntry struct {
	Name    string `json:"name"`
	Family  string `json:"family"`
	Context int    `json:"context"`
	Output  int    `json:"output"`
}

type modelsDevEntry = ModelsDevEntry

var (
	modelsDevMu      sync.RWMutex
	modelsDevCache   map[string]ModelsDevEntry // key: lowercase model id
	modelsDevFetched time.Time
)

// modelsDevProvider mirrors the per-provider shape of api.json.
type modelsDevProvider struct {
	Name   string `json:"name"`
	Models map[string]struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Family string `json:"family"`
		Limit  struct {
			Context int `json:"context"`
			Output  int `json:"output"`
		} `json:"limit"`
	} `json:"models"`
}

// LoadModelsDevCatalog fetches (or reuses) the models.dev catalog as a
// lowercase-id-keyed map. On fetch failure it returns the stale cache (if
// any) or an error.
func LoadModelsDevCatalog(client *http.Client) (map[string]modelsDevEntry, error) {
	if client == nil {
		// pony-tail: model package cannot import provider.SafeHTTPClient due to import cycle (provider imports model).
		// Production callers (server.go) always inject s.getHTTPClient(); nil client is exclusively used in isolated unit tests
		// against hardcoded ModelsDevURL ("https://models.dev/api.json").
		client = &http.Client{Timeout: 20 * time.Second}
	}

	modelsDevMu.RLock()
	if modelsDevCache != nil && time.Since(modelsDevFetched) < modelsDevCacheTTL {
		c := modelsDevCache
		modelsDevMu.RUnlock()
		return c, nil
	}
	modelsDevMu.RUnlock()

	req, err := http.NewRequest("GET", ModelsDevURL, nil)
	if err != nil {
		return modelsDevStale(), err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return modelsDevStale(), fmt.Errorf("models.dev fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return modelsDevStale(), fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return modelsDevStale(), err
	}

	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &providers); err != nil {
		return modelsDevStale(), fmt.Errorf("models.dev parse failed: %w", err)
	}

	catalog := make(map[string]modelsDevEntry, 4096)
	for _, p := range providers {
		for id, m := range p.Models {
			key := strings.ToLower(id)
			if _, dup := catalog[key]; dup {
				continue
			}
			catalog[key] = modelsDevEntry{
				Name:    m.Name,
				Family:  m.Family,
				Context: m.Limit.Context,
				Output:  m.Limit.Output,
			}
		}
	}

	modelsDevMu.Lock()
	modelsDevCache = catalog
	modelsDevFetched = time.Now()
	modelsDevMu.Unlock()

	return catalog, nil
}

func modelsDevStale() map[string]modelsDevEntry {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	return modelsDevCache
}

// LookupModelsDev resolves a model id against the catalog: exact lowercase
// match first, then stripped provider prefix (e.g. "openai/gpt-4o" -> "gpt-4o"),
// then safe fuzzy containment (e.g. modelID contains catalog key).
func LookupModelsDev(catalog map[string]ModelsDevEntry, modelID string) (ModelsDevEntry, bool) {
	if len(catalog) == 0 || modelID == "" {
		return ModelsDevEntry{}, false
	}
	lower := strings.ToLower(modelID)
	if e, ok := catalog[lower]; ok {
		return e, true
	}
	// Strip provider prefix (e.g. "openai/gpt-4o" -> "gpt-4o").
	if idx := strings.Index(lower, "/"); idx > 0 {
		if e, ok := catalog[lower[idx+1:]]; ok {
			return e, true
		}
	}
	// Exact stripped dash/space match
	lowerDashed := strings.ReplaceAll(lower, " ", "-")
	if e, ok := catalog[lowerDashed]; ok {
		return e, true
	}
	// Fuzzy: model id contains a catalog id, best = longest match (min length 4 to avoid greedy collisions).
	best, bestLen := ModelsDevEntry{}, 0
	for key, e := range catalog {
		if len(key) <= bestLen || len(key) < 4 {
			continue
		}
		if strings.Contains(lower, key) || strings.Contains(lowerDashed, key) || (len(lower) >= 6 && strings.Contains(key, lower)) {
			best, bestLen = e, len(key)
		}
	}
	if bestLen > 0 {
		return best, true
	}
	return ModelsDevEntry{}, false
}

// LookupModelsDevGlobal resolves a model id or display name against the cached models.dev catalog.
func LookupModelsDevGlobal(identifiers ...string) *ModelMetadata {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	cat := modelsDevCache
	if len(cat) == 0 {
		return nil
	}
	for _, raw := range identifiers {
		if raw == "" {
			continue
		}
		if e, ok := LookupModelsDev(cat, raw); ok {
			primaryID := raw
			if len(identifiers) > 0 && identifiers[0] != "" {
				primaryID = identifiers[0]
			}
			return &ModelMetadata{
				ID:            primaryID,
				DisplayName:   e.Name,
				ContextLength: e.Context,
				MaxOutput:     e.Output,
				Family:        e.Family,
				FromUpstream:  false,
			}
		}
	}
	return nil
}

// SetModelsDevCatalog updates the in-memory models.dev catalog.
func SetModelsDevCatalog(catalog map[string]ModelsDevEntry) {
	modelsDevMu.Lock()
	modelsDevCache = catalog
	modelsDevFetched = time.Now()
	modelsDevMu.Unlock()
}

// GetModelsDevCatalog returns the current in-memory models.dev catalog copy.
func GetModelsDevCatalog() map[string]ModelsDevEntry {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	return modelsDevCache
}

// BackfillFromModelsDev fills missing ContextLength/MaxOutput/Family/
// DisplayName on live-fetched models using the models.dev catalog. Existing
// values are never overwritten.
func BackfillFromModelsDev(models []ModelMetadata, catalog map[string]modelsDevEntry) {
	for i := range models {
		m := &models[i]
		e, ok := LookupModelsDev(catalog, m.ID)
		if !ok && m.DisplayName != "" && m.DisplayName != m.ID {
			e, ok = LookupModelsDev(catalog, m.DisplayName)
		}
		if !ok {
			continue
		}
		if m.DisplayName == "" {
			m.DisplayName = e.Name
		}
		if m.ContextLength == 0 {
			m.ContextLength = e.Context
		}
		if m.MaxOutput == 0 {
			m.MaxOutput = e.Output
		}
		if m.Family == "" {
			m.Family = e.Family
		}
	}
}
