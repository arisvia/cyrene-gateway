package provider

import (
	"sort"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
)

// Account-level quota deadline: the soonest reset among the buckets that still
// have headroom. Buckets that are already exhausted are ignored on purpose —
// their reset date says nothing about value left on the account, and one empty
// bucket with an early reset would otherwise pull a rich account forward.
//
// The deadline is persisted into providerSpecificData so selection can read it
// without calling the provider. It is only as fresh as the last quota fetch
// (panel open or the background sweep), which is the deliberate ceiling here:
// quota is not polled per request.
const (
	quotaDeadlineKey = "quotaDeadline"

	// quotaDeadlineUrgentWindow is how close a reset has to be before an account
	// is considered burning. Resets precede expiry: an account whose bucket
	// refills in under two days has almost nothing to lose by being drained now.
	quotaDeadlineUrgentWindow = 48 * time.Hour

	// quotaDeadlineGroupSlack is how close two deadlines must be to share one
	// rotation group. Deadlines arrive from remote quota APIs and never match to
	// the second, so exact equality would hand one account 100% of the traffic
	// whenever siblings differ by a few minutes. Credits move on a day scale, so
	// a day-wide window groups what a human would call "the same deadline".
	quotaDeadlineGroupSlack = 24 * time.Hour
)

// QuotaBucket is the subset of a provider quota row that deadline aggregation
// needs. It mirrors the wire fields of usage.Quota without importing it, so the
// selection layer stays free of the fetcher package.
type QuotaBucket struct {
	Remaining           float64
	RemainingPercentage float64
	ResetAt             string
	Unlimited           bool
}

// hasHeadroom reports whether a bucket still has credits left, in whichever
// unit the provider reported: some return absolute remainder, others only a
// percentage, so either being positive counts.
func (b QuotaBucket) hasHeadroom() bool {
	return b.Remaining > 0 || b.RemainingPercentage > 0
}

// QuotaDeadline derives the soonest useful reset from a provider's buckets.
// It returns the zero time when no bucket has both headroom and a parseable
// reset, which callers treat as "unknown, do not prioritise".
func QuotaDeadline(buckets []QuotaBucket, now time.Time) time.Time {
	var soonest time.Time
	for _, b := range buckets {
		if b.Unlimited || !b.hasHeadroom() {
			continue
		}
		t, err := time.Parse(time.RFC3339, b.ResetAt)
		if err != nil {
			continue
		}
		// A reset in the past means the snapshot is stale (the bucket should
		// have refilled already). Keeping it would sort it first forever and pin
		// all traffic to an account whose real state is no longer known.
		if !t.After(now) {
			continue
		}
		if soonest.IsZero() || t.Before(soonest) {
			soonest = t
		}
	}
	return soonest.UTC()
}

// SetQuotaDeadline records the derived deadline on a connection. A zero
// deadline clears the key instead of storing an epoch, so "unknown" and
// "already passed" stay distinguishable.
func SetQuotaDeadline(conn *model.ProviderConnection, deadline time.Time) bool {
	if conn == nil {
		return false
	}
	desired := ""
	if !deadline.IsZero() {
		desired = deadline.UTC().Format(time.RFC3339)
	}
	current, _ := conn.Data.ProviderSpecificData[quotaDeadlineKey].(string)
	if current == desired {
		return false
	}
	if desired == "" {
		if conn.Data.ProviderSpecificData == nil {
			return false // nothing stored, nothing to clear
		}
		conn.Data.ProviderSpecificData[quotaDeadlineKey] = nil
		return true
	}
	if conn.Data.ProviderSpecificData == nil {
		conn.Data.ProviderSpecificData = make(map[string]any)
	}
	conn.Data.ProviderSpecificData[quotaDeadlineKey] = desired
	return true
}

// QuotaDeadlinePatch returns the providerSpecificData patch that persists the
// deadline, or nil when the connection already carries it. A nil value removes
// the key, matching UpdateConnectionProviderData's delete convention.
func QuotaDeadlinePatch(conn *model.ProviderConnection, deadline time.Time) map[string]any {
	if !SetQuotaDeadline(conn, deadline) {
		return nil
	}
	value := any(nil)
	if v, ok := conn.Data.ProviderSpecificData[quotaDeadlineKey].(string); ok && v != "" {
		value = v
	}
	return map[string]any{quotaDeadlineKey: value}
}

// quotaDeadlineOf reads the stored deadline, returning the zero time when the
// key is absent or unparseable.
func quotaDeadlineOf(c *model.ProviderConnection) time.Time {
	raw, _ := c.Data.ProviderSpecificData[quotaDeadlineKey].(string)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// effectiveDeadline is the deadline ordering may act on: a stored instant that
// has already passed means the snapshot is stale (that bucket should have
// refilled), so it degrades to unknown instead of sorting first forever.
func effectiveDeadline(c *model.ProviderConnection, now time.Time) time.Time {
	d := quotaDeadlineOf(c)
	if !d.IsZero() && !d.After(now) {
		return time.Time{}
	}
	return d
}

// orderByQuotaDeadline reorders a rotation pool so accounts whose quota expires
// soonest come first. Accounts with no known deadline keep their relative order
// at the back: unknown urgency must never outrank a measured one.
//
// This only permutes within the already-selected pool, so priority tiers, the
// OAuth prefix, and session affinity bindings are all untouched.
func orderByQuotaDeadline(pool []*model.ProviderConnection, now time.Time) []*model.ProviderConnection {
	if len(pool) < 2 {
		return pool
	}
	type ranked struct {
		conn     *model.ProviderConnection
		deadline time.Time
	}
	ranks := make([]ranked, len(pool))
	any := false
	for i, c := range pool {
		d := effectiveDeadline(c, now)
		ranks[i] = ranked{conn: c, deadline: d}
		if !d.IsZero() {
			any = true
		}
	}
	if !any {
		return pool
	}
	sort.SliceStable(ranks, func(i, j int) bool {
		iKnown, jKnown := !ranks[i].deadline.IsZero(), !ranks[j].deadline.IsZero()
		if iKnown != jKnown {
			return iKnown
		}
		if !iKnown {
			return false
		}
		return ranks[i].deadline.Before(ranks[j].deadline)
	})
	out := make([]*model.ProviderConnection, len(pool))
	for i := range ranks {
		out[i] = ranks[i].conn
	}
	return out
}

// QuotaDeadlineIsUrgent reports whether a deadline falls inside the urgent
// window, used by the panel to explain why an account is being drained first.
func QuotaDeadlineIsUrgent(deadline, now time.Time) bool {
	if deadline.IsZero() {
		return false
	}
	return deadline.Sub(now) <= quotaDeadlineUrgentWindow
}
