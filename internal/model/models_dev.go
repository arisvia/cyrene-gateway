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

// modelsDevEntry is the subset of a models.dev model record we need.
type modelsDevEntry struct {
	Name    string
	Family  string
	Context int
	Output  int
}

var (
	modelsDevMu      sync.Mutex
	modelsDevCache   map[string]modelsDevEntry // key: lowercase model id
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
		client = &http.Client{Timeout: 20 * time.Second}
	}

	modelsDevMu.Lock()
	if modelsDevCache != nil && time.Since(modelsDevFetched) < modelsDevCacheTTL {
		c := modelsDevCache
		modelsDevMu.Unlock()
		return c, nil
	}
	modelsDevMu.Unlock()

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
	modelsDevMu.Lock()
	defer modelsDevMu.Unlock()
	return modelsDevCache
}

// LookupModelsDev resolves a model id against the catalog: exact lowercase
// match first, then fuzzy containment (id appears inside the model id or vice
// versa) to handle prefixed ids like "openai/gpt-4o" vs "gpt-4o".
func LookupModelsDev(catalog map[string]modelsDevEntry, modelID string) (modelsDevEntry, bool) {
	if len(catalog) == 0 || modelID == "" {
		return modelsDevEntry{}, false
	}
	lower := strings.ToLower(modelID)
	if e, ok := catalog[lower]; ok {
		return e, true
	}
	// Strip provider prefix (e.g. "openai/gpt-4o" → "gpt-4o").
	if idx := strings.Index(lower, "/"); idx > 0 {
		if e, ok := catalog[lower[idx+1:]]; ok {
			return e, true
		}
	}
	// Fuzzy: model id contains a catalog id (or vice versa), best = longest.
	best, bestLen := modelsDevEntry{}, 0
	for key, e := range catalog {
		if len(key) <= bestLen {
			continue
		}
		if strings.Contains(lower, key) || strings.Contains(key, lower) {
			best, bestLen = e, len(key)
		}
	}
	if bestLen > 0 {
		return best, true
	}
	return modelsDevEntry{}, false
}

// BackfillFromModelsDev fills missing ContextLength/MaxOutput/Family/
// DisplayName on live-fetched models using the models.dev catalog. Existing
// values are never overwritten.
func BackfillFromModelsDev(models []ModelMetadata, catalog map[string]modelsDevEntry) {
	for i := range models {
		m := &models[i]
		e, ok := LookupModelsDev(catalog, m.ID)
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
