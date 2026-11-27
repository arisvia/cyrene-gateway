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
	"slices"
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
	// ReleaseDate is models.dev's "release_date" (YYYY-MM-DD). It is the only
	// trustworthy publication stamp we have, and it is what backs the OpenAI
	// `created` field — never synthesized from the fetch time.
	ReleaseDate string `json:"release_date"`
}

var (
	modelsDevMu      sync.RWMutex
	modelsDevCache   map[string]ModelsDevEntry // key: lowercase model id
	modelsDevFetched time.Time
	// modelsDevFetchMu serializes catalog downloads (single-flight); it is never
	// held while modelsDevMu is read for anything but a snapshot.
	modelsDevFetchMu sync.Mutex
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
		ReleaseDate string `json:"release_date"`
	} `json:"models"`
}

// LoadModelsDevCatalog fetches (or reuses) the models.dev catalog as a
// lowercase-id-keyed map. On fetch failure it returns the stale cache (if
// any) or an error.
func LoadModelsDevCatalog(client *http.Client) (map[string]ModelsDevEntry, time.Time, error) {
	if client == nil {
		// Fail closed. model cannot import provider.SafeHTTPClient (provider
		// imports model), so the only safe choices are "a caller-supplied
		// SSRF-pinned client" or "no request at all" — never a bare
		// http.Client, which would silently bypass AGENTS.md rule 3 for any
		// future caller that forgets to inject one.
		return nil, time.Time{}, fmt.Errorf("models.dev catalog requires an SSRF-pinned HTTP client")
	}

	// One download at a time. The TTL test below is otherwise a TOCTOU: N callers
	// that all observe an expired catalog — a boot with several providers syncing,
	// or a dashboard polling stale rows — each pull the whole 20 MB file. Queuing
	// here lets the followers find the leader's result already installed instead.
	modelsDevFetchMu.Lock()
	defer modelsDevFetchMu.Unlock()

	if !ModelsDevCatalogExpired() {
		modelsDevMu.RLock()
		c, at := modelsDevCache, modelsDevFetched
		modelsDevMu.RUnlock()
		return c, at, nil
	}

	req, err := http.NewRequest("GET", ModelsDevURL, nil)
	if err != nil {
		return modelsDevStale(), ModelsDevFetchedAt(), err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return modelsDevStale(), ModelsDevFetchedAt(), fmt.Errorf("models.dev fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return modelsDevStale(), ModelsDevFetchedAt(), fmt.Errorf("models.dev returned status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024))
	if err != nil {
		return modelsDevStale(), ModelsDevFetchedAt(), err
	}

	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(raw, &providers); err != nil {
		return modelsDevStale(), ModelsDevFetchedAt(), fmt.Errorf("models.dev parse failed: %w", err)
	}

	catalog := make(map[string]ModelsDevEntry, 4096)
	// Provider order must not decide which duplicate wins: models.dev publishes the
	// same id under several providers (e.g. gpt-4o under openai and azure-openai)
	// with different release dates, and ranging the map directly kept a random one
	// per fetch — so the published `created` changed every refresh.
	providerIDs := make([]string, 0, len(providers))
	for id := range providers {
		providerIDs = append(providerIDs, id)
	}
	slices.Sort(providerIDs)
	for _, pid := range providerIDs {
		p := providers[pid]
		for id, m := range p.Models {
			key := strings.ToLower(id)
			if _, dup := catalog[key]; dup {
				continue
			}
			catalog[key] = ModelsDevEntry{
				Name:        m.Name,
				Family:      m.Family,
				Context:     m.Limit.Context,
				Output:      m.Limit.Output,
				ReleaseDate: m.ReleaseDate,
			}
		}
	}

	fetched := time.Now()
	modelsDevMu.Lock()
	modelsDevCache = catalog
	modelsDevFetched = fetched
	modelsDevMu.Unlock()

	// The stamp is returned with the catalog it belongs to: a caller persisting the
	// pair used to re-read ModelsDevFetchedAt() afterwards and could store this
	// catalog under a concurrent fetch's time.
	return catalog, fetched, nil
}

func modelsDevStale() map[string]ModelsDevEntry {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	return modelsDevCache
}

// modelsDevMatch records HOW a catalog hit was obtained, because some consumers
// may only accept certainty. Containment matching resolves opaque provider ids
// and human-readable names, but "grok-4" contained in the catalog key
// "grok-4-fast-reasoning" is a *different model* — its release date would be a
// fabrication, and its tie against another same-length key used to be decided by
// Go's randomized map order (probed: 261 vs 39 across 300 lookups).
type modelsDevMatch int

const (
	matchNone modelsDevMatch = iota
	// matchExact: the identifier equals a catalog key, after stripping a provider
	// prefix or normalising spaces to dashes.
	matchExact
	// matchNameExact: the identifier equals one catalog entry's display name.
	matchNameExact
	// matchContains: one string contains the other. Names and limits only.
	matchContains
)

// LookupModelsDev resolves a model id against the catalog: exact lowercase
// match first, then stripped provider prefix (e.g. "openai/gpt-4o" -> "gpt-4o"),
// then safe fuzzy containment (e.g. modelID contains catalog key).
func LookupModelsDev(catalog map[string]ModelsDevEntry, modelID string) (ModelsDevEntry, bool) {
	entry, match := lookupModelsDev(catalog, modelID)
	return entry, match != matchNone
}

func lookupModelsDev(catalog map[string]ModelsDevEntry, modelID string) (ModelsDevEntry, modelsDevMatch) {
	if len(catalog) == 0 || modelID == "" {
		return ModelsDevEntry{}, matchNone
	}
	lower := strings.ToLower(modelID)
	if e, ok := catalog[lower]; ok {
		return e, matchExact
	}
	// Strip provider prefix (e.g. "openai/gpt-4o" -> "gpt-4o").
	if idx := strings.Index(lower, "/"); idx > 0 {
		if e, ok := catalog[lower[idx+1:]]; ok {
			return e, matchExact
		}
	}
	// Exact stripped dash/space match
	lowerDashed := strings.ReplaceAll(lower, " ", "-")
	if e, ok := catalog[lowerDashed]; ok {
		return e, matchExact
	}
	// Fuzzy: model id contains a catalog id, best = longest match (min length 4 to
	// avoid greedy collisions). Length alone is not a total order, so equal-length
	// candidates break lexically; iterating a map must never decide the winner.
	best, bestKey := ModelsDevEntry{}, ""
	for key, e := range catalog {
		if len(key) < 4 {
			continue
		}
		if !strings.Contains(lower, key) && !strings.Contains(lowerDashed, key) &&
			!(len(lower) >= 6 && strings.Contains(key, lower)) {
			continue
		}
		if len(key) > len(bestKey) || (len(key) == len(bestKey) && key < bestKey) {
			best, bestKey = e, key
		}
	}
	if bestKey != "" {
		return best, matchContains
	}
	// Display-name resolution: opaque provider ids (e.g. Qoder's "kmodel_latest")
	// never match a models.dev key, but the provider's human-readable display name
	// ("Kimi-K3") is a real model name. When the query itself is a name, match it
	// against each entry's Name field. Exact normalized-name hits win outright;
	// otherwise the longest containment (min 4 chars) wins, ties broken by the
	// catalog key so the result does not depend on map order.
	nameNorm := lowerDashed
	bestName, bestNameKey, bestNameLen := ModelsDevEntry{}, "", 0
	for key, e := range catalog {
		if e.Name == "" {
			continue
		}
		en := strings.ReplaceAll(strings.ToLower(e.Name), " ", "-")
		if en == nameNorm {
			return e, matchNameExact
		}
		if len(en) >= 4 &&
			(strings.Contains(nameNorm, en) || (len(nameNorm) >= 6 && strings.Contains(en, nameNorm))) &&
			(len(en) > bestNameLen || (len(en) == bestNameLen && key < bestNameKey)) {
			bestName, bestNameKey, bestNameLen = e, key, len(en)
		}
	}
	if bestNameLen > 0 {
		return bestName, matchContains
	}
	return ModelsDevEntry{}, matchNone
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
		e, match := lookupModelsDev(cat, raw)
		if match == matchNone {
			continue
		}
		primaryID := raw
		if len(identifiers) > 0 && identifiers[0] != "" {
			primaryID = identifiers[0]
		}
		// `created` is the one field a containment match must not supply: the
		// catalog key that merely contains the query can belong to a different
		// model ("grok-4" inside "grok-4-fast-reasoning"), and a plausible-looking
		// release date for the wrong model is indistinguishable downstream. Omit it
		// and the field disappears; publish it wrongly and it silently lies.
		var created int64
		if match != matchContains {
			created = releaseDateToEpoch(e.ReleaseDate)
		}
		return &ModelMetadata{
			ID:            primaryID,
			DisplayName:   e.Name,
			ContextLength: e.Context,
			MaxOutput:     e.Output,
			Family:        e.Family,
			Created:       created,
			FromUpstream:  false,
		}
	}
	return nil
}

// releaseDateToEpoch converts a models.dev "release_date" into the Unix stamp the
// OpenAI `created` field means. Unknown dates and the catalog's 1970-01-01
// placeholder both yield 0, which every consumer omits — an absent stamp beats a
// invented one.
func releaseDateToEpoch(date string) int64 {
	if date == "" {
		return 0
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil || t.Year() <= 1970 {
		return 0
	}
	return t.Unix()
}

// SetModelsDevCatalog updates the in-memory models.dev catalog, stamped as just
// fetched. Restoring persisted entries must use SetModelsDevCatalogAt instead.
func SetModelsDevCatalog(catalog map[string]ModelsDevEntry) {
	SetModelsDevCatalogAt(catalog, time.Now())
}

// SetModelsDevCatalogAt installs a catalog together with the instant it was
// fetched. The stamp has to survive the restart that reads it back:
// LoadModelsDevCatalog skips the network while the cache is younger than
// modelsDevCacheTTL, so priming memory from disk with "now" reset the clock on
// every boot and the catalog was never refreshed from upstream again — an old,
// dateless catalog (one written before release dates were parsed) reproduced
// itself indefinitely, which emptied `created` on every installed gateway.
func SetModelsDevCatalogAt(catalog map[string]ModelsDevEntry, fetchedAt time.Time) {
	modelsDevMu.Lock()
	modelsDevCache = catalog
	modelsDevFetched = fetchedAt
	modelsDevMu.Unlock()
}

// modelsDevCachePayload is the persisted catalog layout: entries plus the fetch
// stamp.
type modelsDevCachePayload struct {
	FetchedAt time.Time                 `json:"fetched_at"`
	Entries   map[string]ModelsDevEntry `json:"entries"`
}

// EncodeModelsDevCache serializes the catalog for KV storage.
func EncodeModelsDevCache(entries map[string]ModelsDevEntry, fetchedAt time.Time) (string, error) {
	data, err := json.Marshal(modelsDevCachePayload{FetchedAt: fetchedAt, Entries: entries})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DecodeModelsDevCache parses a persisted catalog envelope and rejects missing
// entries or fetch timestamps so stale/unrecognized cache layouts are refreshed cleanly.
func DecodeModelsDevCache(raw string) (map[string]ModelsDevEntry, time.Time, error) {
	var payload modelsDevCachePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, time.Time{}, err
	}
	if payload.Entries == nil || payload.FetchedAt.IsZero() {
		return nil, time.Time{}, fmt.Errorf("malformed models.dev cache payload: fetched_at or entries missing")
	}
	return payload.Entries, payload.FetchedAt, nil
}

// ModelsDevFetchedAt reports when the in-memory catalog was last fetched from
// upstream. Persistence must store this instant rather than the wall clock, or
// re-saving a cache that was merely reused would rewrite its age and make it
// immortal again.
func ModelsDevFetchedAt() time.Time {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	return modelsDevFetched
}

// ModelsDevCatalogExpired reports the condition under which loading the catalog
// would go to the network: nothing in memory, or a restored/loaded copy whose age
// already passed the TTL. Callers use it to decide whether warming the catalog is
// worth a request at all.
func ModelsDevCatalogExpired() bool {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	return modelsDevCache == nil || time.Since(modelsDevFetched) >= modelsDevCacheTTL
}

// GetModelsDevCatalog returns the live in-memory models.dev catalog. The map is
// shared, not copied — a 20 MB catalog copied on every admin panel read would be
// absurd — so callers must treat it as read-only. That holds today because
// writers replace the whole map under modelsDevMu (SetModelsDevCatalog) rather
// than mutating entries in place; any future in-place edit must copy first.
func GetModelsDevCatalog() map[string]ModelsDevEntry {
	modelsDevMu.RLock()
	defer modelsDevMu.RUnlock()
	return modelsDevCache
}
