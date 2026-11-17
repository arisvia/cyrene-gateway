package provider

import (
	"encoding/json"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
)

// TestRegistryOrderPrefersPriorityThenID pins the comparator the router and the
// client model list share: registry priority first, id only as a tie-break, and
// providers with no declared priority last. Both surfaces must agree or a bare
// model name would be advertised by one provider and answered by another.
func TestRegistryOrderPrefersPriorityThenID(t *testing.T) {
	// Precondition: the pair below must really exercise the priority rule, not
	// luck out on the lexical tie-break.
	if registryPriority("opencode") >= registryPriority("deepseek") {
		t.Fatalf("test premise broken: opencode(%d) should outrank deepseek(%d)",
			registryPriority("opencode"), registryPriority("deepseek"))
	}
	if got := RegistryOrder("deepseek", "opencode"); got <= 0 {
		t.Errorf("lower priority number must sort first: order(%q,%q)=%d, want positive", "deepseek", "opencode", got)
	}
	if got := RegistryOrder("no-such-provider", "deepseek"); got <= 0 {
		t.Errorf("a provider with no declared priority must sort last, got %d", got)
	}
	if got := RegistryOrder("aaa", "bbb"); got >= 0 {
		t.Errorf("equal priority must break by id, got %d", got)
	}
	if got := RegistryOrder("same", "same"); got != 0 {
		t.Errorf("self-comparison must be 0, got %d", got)
	}
}

// TestQoderCNRanksAboveIntlForBareNames: both Qoder deployments publish the same
// qmodel_* catalogue, and bare model names resolve in registry order. The Intl
// account spends minutes per request in the upstream queue (10605) while the CN
// pool answers in seconds, so qoder-cn ranks ahead — an operator or client typing
// `qmodel_latest_preview` reaches the pool that actually replies. Naming
// `qoder/...` explicitly still selects Intl, and the client list order matches.
func TestQoderCNRanksAboveIntlForBareNames(t *testing.T) {
	if registryPriority("qoder-cn") >= registryPriority("qoder") {
		t.Fatalf("qoder-cn must outrank qoder: got cn=%d intl=%d", registryPriority("qoder-cn"), registryPriority("qoder"))
	}
	if got := RegistryOrder("qoder", "qoder-cn"); got <= 0 {
		t.Errorf("order(qoder, qoder-cn)=%d, want positive so bare qmodel_* names resolve to the CN pool", got)
	}
}

// TestResolveModelBareNameIsDeterministic: one bare model id cached by two
// providers used to resolve through Go's randomized map iteration, so the same
// request string landed on a different upstream from call to call — different
// billing, different prompt handling, and a different provider than /v1/models
// advertises (probed on production: 31 bare ids are shared, 453 queued Qoder
// requests mixed with 65 completed ones in a single hour).
func TestResolveModelBareNameIsDeterministic(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	// The lower-priority-numbered provider is written last on purpose: insertion
	// order must not be what decides the answer.
	for _, p := range []string{"opencode", "openai"} {
		conn := &model.ProviderConnection{
			ID:       "conn-" + p,
			Provider: p,
			AuthType: "api-key",
			IsActive: true,
			Data:     model.ConnectionData{APIKey: "sk-test"},
		}
		if err := database.CreateConnection(conn); err != nil {
			t.Fatalf("create connection %s: %v", p, err)
		}
		raw, _ := json.Marshal(model.CachedModels{Models: []model.ModelMetadata{{ID: "shared-reasoner", DisplayName: "Shared Reasoner"}}})
		if err := database.KVSet(model.KVScopeProviderModelCache, p, string(raw)); err != nil {
			t.Fatalf("seed cache %s: %v", p, err)
		}
	}

	want := "openai"
	if registryPriority(want) >= registryPriority("opencode") {
		t.Fatalf("test premise broken: expected openai to outrank opencode")
	}
	for i := 0; i < 200; i++ {
		info, err := ResolveModel("shared-reasoner", database)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if info.Provider != want {
			t.Fatalf("iteration %d: bare name resolved to %s, want the outranking provider %s", i, info.Provider, want)
		}
	}
}
