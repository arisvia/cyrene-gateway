package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// withCatalog installs a controlled catalog for one test and restores whatever was
// there before: the catalog is process-global, so a test that mutates it without
// restoring leaks into its neighbours.
func withCatalog(t *testing.T, entries map[string]ModelsDevEntry) {
	t.Helper()
	prev, prevAt := GetModelsDevCatalog(), ModelsDevFetchedAt()
	t.Cleanup(func() { SetModelsDevCatalogAt(prev, prevAt) })
	SetModelsDevCatalogAt(entries, time.Now())
}

// TestLookupModelsDevGlobalNeverDatesAFuzzyHit pins the provenance rule for
// `created`: containment matching may borrow a sibling's display name and limits,
// but never its release date. A gateway publishing a date for the wrong model is
// indistinguishable from one publishing the right date, whereas an absent field
// simply says "unknown". Observed on production: `grok-4` inherited
// `grok-4-fast-reasoning`'s release date through reverse containment.
func TestLookupModelsDevGlobalNeverDatesAFuzzyHit(t *testing.T) {
	withCatalog(t, map[string]ModelsDevEntry{
		"grok-4-fast-reasoning": {Name: "Grok 4 Fast Reasoning", Context: 256000, Output: 8192, ReleaseDate: "2025-11-13"},
	})

	fuzzy := LookupModelsDevGlobal("grok-4")
	if fuzzy == nil {
		t.Fatal("expected the containment hit to keep enriching name and limits")
	}
	if fuzzy.DisplayName != "Grok 4 Fast Reasoning" || fuzzy.ContextLength != 256000 {
		t.Errorf("unexpected fuzzy enrichment: %+v", fuzzy)
	}
	if fuzzy.Created != 0 {
		t.Errorf("containment match published created=%d; only exact matches may date a model", fuzzy.Created)
	}

	exact := LookupModelsDevGlobal("grok-4-fast-reasoning")
	if exact == nil {
		t.Fatal("exact id must resolve")
	}
	want := time.Date(2025, 11, 13, 0, 0, 0, 0, time.UTC).Unix()
	if exact.Created != want {
		t.Errorf("exact match created=%d, want %d", exact.Created, want)
	}
}

// TestLookupModelsDevTieBreakIsDeterministic: length alone is not a total order, so
// two catalog keys of equal length that both contain the query used to let Go's
// randomized map iteration pick the winner (probed: the same query flipped 261/39
// over 300 runs). Equal-length candidates must break lexically, and repeatedly.
func TestLookupModelsDevTieBreakIsDeterministic(t *testing.T) {
	withCatalog(t, map[string]ModelsDevEntry{
		"grok-4-plus": {Name: "Grok 4 Plus", ReleaseDate: "2025-02-02"},
		"grok-4-mini": {Name: "Grok 4 Mini", ReleaseDate: "2025-01-01"},
	})
	for i := 0; i < 200; i++ {
		got := LookupModelsDevGlobal("grok-4")
		if got == nil {
			t.Fatalf("iteration %d: no match", i)
		}
		if got.DisplayName != "Grok 4 Mini" {
			t.Fatalf("iteration %d: winner=%q, want the lexicographically smallest key", i, got.DisplayName)
		}
		if got.Created != 0 {
			t.Fatalf("iteration %d: containment match published created=%d", i, got.Created)
		}
	}
}

// TestMergeMetadataReusesCachedCreated: when the live catalog has nothing for a
// model but its cached row carries a date, keep the date — it was verified by the
// enrichment pass that wrote the row. A catalog hit must still take precedence.
func TestMergeMetadataReusesCachedCreated(t *testing.T) {
	withCatalog(t, map[string]ModelsDevEntry{
		"dated-model": {Name: "Dated Model", ReleaseDate: "2024-05-06"},
	})
	cachedDate := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC).Unix()

	if got := MergeMetadata("undated-model", nil, &ModelMetadata{ID: "undated-model", Created: cachedDate}); got.Created != cachedDate {
		t.Errorf("created=%d, want the cached stamp %d", got.Created, cachedDate)
	}

	live := MergeMetadata("dated-model", nil, &ModelMetadata{ID: "dated-model", Created: cachedDate})
	if want := time.Date(2024, 5, 6, 0, 0, 0, 0, time.UTC).Unix(); live.Created != want {
		t.Errorf("catalog hit created=%d, want %d (cached must not win over a live match)", live.Created, want)
	}
}

// TestLoadModelsDevCatalogFetchesOnce: the TTL check was a TOCTOU, so N callers
// arriving with an expired catalog each downloaded the whole 20 MB file. One
// download must serve them all, and every caller must receive the catalog paired
// with the stamp it was cached under.
func TestLoadModelsDevCatalogFetchesOnce(t *testing.T) {
	prev, prevAt := GetModelsDevCatalog(), ModelsDevFetchedAt()
	t.Cleanup(func() { SetModelsDevCatalogAt(prev, prevAt) })
	orig := ModelsDevURL
	t.Cleanup(func() { ModelsDevURL = orig })

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"openai": map[string]any{
				"models": map[string]any{
					"gpt-4o": map[string]any{"id": "gpt-4o", "name": "GPT-4o", "release_date": "2024-05-13"},
				},
			},
		})
	}))
	defer srv.Close()
	ModelsDevURL = srv.URL

	// Expire the catalog the way a day-old process would find it.
	modelsDevMu.Lock()
	modelsDevCache, modelsDevFetched = nil, time.Time{}
	modelsDevMu.Unlock()

	const callers = 8
	var wg sync.WaitGroup
	stamps := make([]time.Time, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cat, at, err := LoadModelsDevCatalog(srv.Client())
			if err != nil {
				t.Errorf("caller %d: %v", i, err)
				return
			}
			if len(cat) == 0 {
				t.Errorf("caller %d: empty catalog", i)
			}
			stamps[i] = at
		}(i)
	}
	wg.Wait()

	if got := hits.Load(); got != 1 {
		t.Errorf("catalog downloaded %d times, want 1", got)
	}
	for i, at := range stamps {
		if at.IsZero() {
			t.Fatalf("caller %d got a zero fetch stamp", i)
		}
		if !at.Equal(stamps[0]) {
			t.Fatalf("caller %d stamp %v differs from %v: catalog and age must arrive as one pair", i, at, stamps[0])
		}
	}
}
