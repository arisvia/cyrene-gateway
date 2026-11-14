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
		// Fail closed. model cannot import provider.SafeHTTPClient (provider
		// imports model), so the only safe choices are "a caller-supplied
		// SSRF-pinned client" or "no request at all" — never a bare
		// http.Client, which would silently bypass AGENTS.md rule 3 for any
		// future caller that forgets to inject one.
		return nil, fmt.Errorf("models.dev catalog requires an SSRF-pinned HTTP client")
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
	// Display-name resolution: opaque provider ids (e.g. Qoder's "kmodel_latest")
	// never match a models.dev key, but the provider's human-readable display name
	// ("Kimi-K3") is a real model name. When the query itself is a name, match it
	// against each entry's Name field. Exact normalized-name hits win outright;
	// otherwise the longest containment (min 4 chars) wins.
	nameNorm := lowerDashed
	bestName, bestNameLen := ModelsDevEntry{}, 0
	for _, e := range catalog {
		if e.Name == "" {
			continue
		}
		en := strings.ReplaceAll(strings.ToLower(e.Name), " ", "-")
		if en == nameNorm {
			return e, true
		}
		if len(en) >= 4 && len(en) > bestNameLen &&
			(strings.Contains(nameNorm, en) || (len(nameNorm) >= 6 && strings.Contains(en, nameNorm))) {
			bestName, bestNameLen = e, len(en)
		}
	}
	if bestNameLen > 0 {
		return bestName, true
	}
	return ModelsDevEntry{}, false
}

// ModelsDevOutputLimit returns the maximum output length models.dev advertises for
// this model, or 0 when nothing is known. Only exact identifier matches count here:
// the value is used to cap a request, and a fuzzy hit would silently shorten answers
// for a model whose real ceiling is higher — a wrong cap is invisible, a rejected
// request is not.
func ModelsDevOutputLimit(providerID, modelID string) int {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	cat := modelsDevCache
	if len(cat) == 0 {
		return 0
	}
	for _, raw := range []string{providerID + "/" + modelID, modelID} {
		lower := strings.ToLower(raw)
		if e, ok := cat[lower]; ok && e.Output > 0 {
			return e.Output
		}
		if idx := strings.Index(lower, "/"); idx > 0 {
			if e, ok := cat[lower[idx+1:]]; ok && e.Output > 0 {
				return e.Output
			}
		}
		if dashed := strings.ReplaceAll(lower, " ", "-"); dashed != lower {
			if e, ok := cat[dashed]; ok && e.Output > 0 {
				return e.Output
			}
		}
	}
	return 0
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
