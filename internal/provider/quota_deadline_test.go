package provider

import (
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

func deadlineConn(id string, priority int, deadline time.Time) model.ProviderConnection {
	c := model.ProviderConnection{ID: id, Provider: "codebuddy", AuthType: "oauth", Priority: priority}
	c.IsActive = true
	c.Data.ProviderSpecificData = map[string]any{}
	if !deadline.IsZero() {
		c.Data.ProviderSpecificData[quotaDeadlineKey] = deadline.UTC().Format(time.RFC3339)
	}
	return c
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("bad fixture time %q: %v", s, err)
	}
	return ts
}

// Exhaustible credits are lost on reset, so once an account is inside the
// urgent window it must absorb traffic even when a sibling has more headroom.
func TestPickFromPoolPrefersSoonestDeadline(t *testing.T) {
	credentialRoundRobin.reset()
	soon := time.Now().Add(6 * time.Hour)
	conns := []model.ProviderConnection{
		deadlineConn("later", 0, soon.Add(10*24*time.Hour)),
		deadlineConn("sooner", 0, soon),
	}
	for i := range 6 {
		got := SelectCredential(conns, "m", nil)
		if got == nil || got.ID != "sooner" {
			t.Fatalf("turn %d: picked %v, want sooner", i, got)
		}
	}
}

// The soonest group rotates internally, so drains stay balanced.
func TestPickFromPoolRotatesWithinUrgentGroup(t *testing.T) {
	credentialRoundRobin.reset()
	same := time.Now().Add(6 * time.Hour)
	conns := []model.ProviderConnection{
		deadlineConn("a", 0, same),
		deadlineConn("b", 0, same),
		deadlineConn("c", 0, same.Add(10*24*time.Hour)),
	}
	seen := map[string]int{}
	for range 6 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["a"] != 3 || seen["b"] != 3 || seen["c"] != 0 {
		t.Fatalf("urgent group must rotate over a,b only, got %v", seen)
	}
}

// An account with no measured deadline must never outrank one that has it.
func TestPickFromPoolUnknownDeadlineSortsLast(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("unknown", 0, time.Time{}),
		deadlineConn("known", 0, time.Now().Add(6*time.Hour)),
	}
	for i := range 4 {
		if got := SelectCredential(conns, "m", nil); got.ID != "known" {
			t.Fatalf("turn %d: picked %v, want known", i, got)
		}
	}
}

// With no deadlines at all, behaviour must be the previous whole-pool rotation.
func TestPickFromPoolNoDeadlinesKeepsRotation(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("x", 0, time.Time{}),
		deadlineConn("y", 0, time.Time{}),
		deadlineConn("z", 0, time.Time{}),
	}
	seen := map[string]int{}
	for range 9 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["x"] != 3 || seen["y"] != 3 || seen["z"] != 3 {
		t.Fatalf("expected even whole-pool rotation, got %v", seen)
	}
}

// Priority tiers still win over urgency: an urgent backup account must not
// steal traffic from a healthy higher-priority account.
func TestQuotaDeadlineNeverCrossesPriorityTier(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("primary", 0, time.Now().Add(20*24*time.Hour)),
		deadlineConn("backup", 5, time.Now().Add(4*time.Hour)),
	}
	for i := range 4 {
		if got := SelectCredential(conns, "m", nil); got.ID != "primary" {
			t.Fatalf("turn %d: picked %v, priority must not be traded", i, got)
		}
	}
}

// Session affinity outranks urgency for an in-pool account: moving a live
// conversation would discard the warm upstream prefix cache, and the urgent
// account is still reachable by every new session.
func TestStickyKeepsBindingDespiteUrgency(t *testing.T) {
	credentialRoundRobin.reset()
	ResetSessionAffinity()
	t.Cleanup(ResetSessionAffinity)

	conns := []model.ProviderConnection{
		deadlineConn("pinned", 0, time.Now().Add(20*24*time.Hour)),
		deadlineConn("urgent", 0, time.Now().Add(4*time.Hour)),
	}
	first := SelectCredentialSticky(conns, "m", nil, nil, "conv-1")
	if first == nil {
		t.Fatal("first turn returned nil")
	}
	for i := range 4 {
		if got := SelectCredentialSticky(conns, "m", nil, nil, "conv-1"); got.ID != first.ID {
			t.Fatalf("turn %d: conversation migrated from %s to %s", i, first.ID, got.ID)
		}
	}
}

// A new conversation, by contrast, must land on the urgent account.
func TestStickyNewSessionPrefersUrgent(t *testing.T) {
	credentialRoundRobin.reset()
	ResetSessionAffinity()

	conns := []model.ProviderConnection{
		deadlineConn("later", 0, time.Now().Add(20*24*time.Hour)),
		deadlineConn("urgent", 0, time.Now().Add(4*time.Hour)),
	}
	for _, key := range []string{"conv-a", "conv-b", "conv-c"} {
		if got := SelectCredentialSticky(conns, "m", nil, nil, key); got.ID != "urgent" {
			t.Fatalf("%s: picked %s, want urgent", key, got.ID)
		}
	}
}

// Aggregation rules: skip exhausted and unlimited buckets, take the earliest.
func TestQuotaDeadlineAggregation(t *testing.T) {
	now := mustParse(t, "2026-11-01T00:00:00Z")
	soon := mustParse(t, "2026-11-07T08:00:00Z")
	later := mustParse(t, "2026-11-29T08:00:00Z")

	got := QuotaDeadline([]QuotaBucket{
		{Remaining: 100, ResetAt: later.Format(time.RFC3339)},
		{Remaining: 0, ResetAt: "2026-11-05T00:00:00Z"}, // exhausted: ignored
		{Remaining: 50, ResetAt: soon.Format(time.RFC3339)},
		{Remaining: 0, ResetAt: "", Unlimited: true},
	}, now)
	if !got.Equal(soon) {
		t.Fatalf("deadline = %s, want %s", got, soon)
	}

	if !QuotaDeadline([]QuotaBucket{{Remaining: 0, ResetAt: "2026-11-05T00:00:00Z"}}, now).IsZero() {
		t.Fatal("an all-exhausted account must report no deadline")
	}
}

// A percentage-only bucket (no absolute remainder) must still count: several
// providers report quota purely as a percentage.
func TestQuotaDeadlineAcceptsPercentageOnlyBucket(t *testing.T) {
	now := mustParse(t, "2026-11-01T00:00:00Z")
	soon := mustParse(t, "2026-11-07T08:00:00Z")

	got := QuotaDeadline([]QuotaBucket{
		{Remaining: 0, RemainingPercentage: 90, ResetAt: soon.Format(time.RFC3339)},
	}, now)
	if !got.Equal(soon) {
		t.Fatalf("percentage-only bucket ignored, got %s want %s", got, soon)
	}
	if !QuotaDeadline([]QuotaBucket{{RemainingPercentage: 0, ResetAt: soon.Format(time.RFC3339)}}, now).IsZero() {
		t.Fatal("a 0%% bucket has no headroom and must be ignored")
	}
}

// A stale (already-passed) reset must not become the deadline: it would sort
// first forever and pin every request to that account.
func TestQuotaDeadlineDropsStaleReading(t *testing.T) {
	now := mustParse(t, "2026-11-10T00:00:00Z")
	if !QuotaDeadline([]QuotaBucket{
		{Remaining: 100, ResetAt: "2026-11-07T08:00:00Z"}, // three days ago
	}, now).IsZero() {
		t.Fatal("a past reset must degrade to unknown, not to most urgent")
	}
}

// A stale stored deadline must read as unknown: it must not be treated as the
// most urgent. With a live sibling present, the sibling owns the urgent group.
func TestStaleStoredDeadlineIsNotUrgent(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("stale", 0, time.Now().Add(-72*time.Hour)), // already passed
		deadlineConn("future", 0, time.Now().Add(6*time.Hour)),  // inside the window
	}
	for i := range 4 {
		if got := SelectCredential(conns, "m", nil); got.ID != "future" {
			t.Fatalf("turn %d: picked %s, a stale deadline must not win", i, got.ID)
		}
	}
}

// When every deadline is stale, ordering must fall back to the plain pool
// rotation rather than inventing an urgent account out of expired data.
func TestAllStaleDeadlinesFallBackToPlainRotation(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("a", 0, time.Now().Add(-72*time.Hour)),
		deadlineConn("b", 0, time.Now().Add(-48*time.Hour)),
	}
	seen := map[string]int{}
	for range 4 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["a"] != 2 || seen["b"] != 2 {
		t.Fatalf("all-stale pool must rotate evenly, got %v", seen)
	}
}

// Deadlines within the slack window form one group: remote APIs never agree to
// the second, and exact matching would give one account 100% of traffic.
func TestUrgentGroupUsesSlackWindow(t *testing.T) {
	credentialRoundRobin.reset()
	base := time.Now().Add(6 * time.Hour).Truncate(time.Second)
	conns := []model.ProviderConnection{
		deadlineConn("a", 0, base),
		deadlineConn("b", 0, base.Add(6*time.Hour)),
		deadlineConn("c", 0, base.Add(10*24*time.Hour)),
	}
	seen := map[string]int{}
	for range 6 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["a"] != 3 || seen["b"] != 3 || seen["c"] != 0 {
		t.Fatalf("near-deadline accounts must share rotation, got %v", seen)
	}
}

// A stored deadline round-trips through the patch and clears cleanly.
func TestQuotaDeadlinePatchRoundTrip(t *testing.T) {
	conn := model.ProviderConnection{ID: "c1"}
	conn.Data.ProviderSpecificData = map[string]any{}
	want := mustParse(t, "2026-11-07T08:00:00Z")

	patch := QuotaDeadlinePatch(&conn, want)
	if patch == nil {
		t.Fatal("expected a patch for a new deadline")
	}
	if got := quotaDeadlineOf(&conn); !got.Equal(want) {
		t.Fatalf("stored %s, want %s", got, want)
	}
	if again := QuotaDeadlinePatch(&conn, want); again != nil {
		t.Fatal("unchanged deadline must not repatch")
	}
	if clear := QuotaDeadlinePatch(&conn, time.Time{}); clear == nil {
		t.Fatal("expected a clearing patch")
	}
	if !quotaDeadlineOf(&conn).IsZero() {
		t.Fatal("deadline should be cleared")
	}
	// Clearing twice is a no-op, not a panic on a nil map.
	if clear := QuotaDeadlinePatch(&model.ProviderConnection{}, time.Time{}); clear != nil {
		t.Fatal("clearing an absent deadline must not patch")
	}
}

// Garbage in the stored value must degrade to "unknown", never to panic.
func TestQuotaDeadlineOfRejectsGarbage(t *testing.T) {
	conn := model.ProviderConnection{}
	conn.Data.ProviderSpecificData = map[string]any{quotaDeadlineKey: "not-a-time"}
	if !quotaDeadlineOf(&conn).IsZero() {
		t.Fatal("unparseable deadline must read as unknown")
	}
}

// Outside the urgent window nothing is about to be wasted, so the rotation must
// stay wide. This is only observable when the deadlines differ enough to form
// distinct groups: without the window guard the near one would absorb all
// traffic, so an even split proves the guard is in force.
func TestDistantDeadlinesKeepFullRotation(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("a", 0, time.Now().Add(29*24*time.Hour)),
		deadlineConn("b", 0, time.Now().Add(30*24*time.Hour)),
	}
	seen := map[string]int{}
	for range 6 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["a"] != 3 || seen["b"] != 3 {
		t.Fatalf("both accounts are months out and must share load, got %v", seen)
	}
}

// Same pool, but the separation is wide enough that slack grouping would split
// them. Both are still outside the window, so the full pool must rotate.
func TestDistantButUngroupedDeadlinesStillShareLoad(t *testing.T) {
	credentialRoundRobin.reset()
	conns := []model.ProviderConnection{
		deadlineConn("near", 0, time.Now().Add(5*24*time.Hour)), // outside 48h
		deadlineConn("far", 0, time.Now().Add(20*24*time.Hour)), // separate group
	}
	seen := map[string]int{}
	for range 6 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["near"] != 3 || seen["far"] != 3 {
		t.Fatalf("neither account is urgent, both must serve, got %v", seen)
	}
}

// Inside the window the narrow group applies, which is the whole point.
func TestUrgentWindowBoundary(t *testing.T) {
	credentialRoundRobin.reset()
	inWindow := time.Now().Add(6 * time.Hour)
	conns := []model.ProviderConnection{
		deadlineConn("urgent", 0, inWindow),
		deadlineConn("distant", 0, time.Now().Add(40*24*time.Hour)),
	}
	seen := map[string]int{}
	for range 6 {
		seen[SelectCredential(conns, "m", nil).ID]++
	}
	if seen["urgent"] != 6 {
		t.Fatalf("an account inside the window must take the traffic, got %v", seen)
	}
}
