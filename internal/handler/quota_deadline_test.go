package handler

import (
	"context"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// persistQuotaDeadline must write the derived deadline to the database, not
// just to the in-memory copy: selection reads connections fresh per request.
func TestPersistQuotaDeadlineWritesThrough(t *testing.T) {
	srv, database := setupTestServer(t)
	conn := model.ProviderConnection{
		ID:       "conn-deadline",
		Provider: "codebuddy",
		AuthType: "oauth",
		IsActive: true,
		Priority: 0,
		Data:     model.ConnectionData{AccessToken: "tok"},
	}
	if err := database.CreateConnection(&conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}

	res := usage.QuotaResult{Quotas: map[string]usage.Quota{
		"soon":  {Used: 10, Total: 100, Remaining: 90, ResetAt: "2026-11-07T08:00:00Z"},
		"later": {Used: 0, Total: 100, Remaining: 100, ResetAt: "2026-12-01T00:00:00Z"},
	}}
	srv.persistQuotaDeadline(&conn, res)

	stored, err := database.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("reload connection: %v", err)
	}
	got, _ := stored.Data.ProviderSpecificData["quotaDeadline"].(string)
	if got != "2026-11-07T08:00:00Z" {
		t.Fatalf("stored deadline = %q, want the soonest bucket", got)
	}
}

// A provider whose buckets are all exhausted must not record a deadline: an
// empty bucket's reset says nothing about value left on the account.
func TestPersistQuotaDeadlineSkipsExhausted(t *testing.T) {
	srv, _ := setupTestServer(t)
	conn := model.ProviderConnection{ID: "c2", Provider: "codebuddy", IsActive: true}
	conn.Data.ProviderSpecificData = map[string]any{}

	srv.persistQuotaDeadline(&conn, usage.QuotaResult{Quotas: map[string]usage.Quota{
		"spent": {Used: 100, Total: 100, Remaining: 0, ResetAt: "2026-11-05T00:00:00Z"},
	}})
	if v, ok := conn.Data.ProviderSpecificData["quotaDeadline"]; ok && v != nil && v != "" {
		t.Fatalf("exhausted account must not get a deadline, got %v", v)
	}
}

// The sweeper must skip providers with no quota API instead of probing them.
func TestSyncAllQuotaDeadlinesSkipsUnsupported(t *testing.T) {
	srv, database := setupTestServer(t)
	conn := model.ProviderConnection{
		ID:       "conn-nosupport",
		Provider: "custom-openai",
		AuthType: "api-key",
		IsActive: true,
		Data:     model.ConnectionData{APIKey: "k"},
	}
	if err := database.CreateConnection(&conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.syncAllQuotaDeadlines(ctx)

	stored, err := database.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if v, ok := stored.Data.ProviderSpecificData["quotaDeadline"]; ok && v != nil && v != "" {
		t.Fatalf("unsupported provider must not get a deadline, got %v", v)
	}
}

// Bucket adaptation: remainder is derived from usage when only that is reported.
func TestQuotaBucketsOfDerivesRemaining(t *testing.T) {
	buckets := quotaBucketsOf(usage.QuotaResult{Quotas: map[string]usage.Quota{
		"usageOnly": {Used: 30, Total: 100, ResetAt: "2026-11-07T08:00:00Z"},
		"unlimited": {Unlimited: true, ResetAt: "2026-11-07T08:00:00Z"},
	}})
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}
	for _, b := range buckets {
		if b.Remaining <= 0 {
			t.Errorf("bucket must retain headroom, got %+v", b)
		}
	}
}

// A failed fetch (empty Quotas + a Message) must leave the stored deadline
// alone. Clearing it would demote a measured account to "unknown" on a single
// transient 500/401/timeout, silently disabling expiry-aware ordering.
func TestPersistQuotaDeadlineKeepsLastKnownOnFailure(t *testing.T) {
	srv, database := setupTestServer(t)
	conn := model.ProviderConnection{
		ID:       "conn-keep",
		Provider: "codebuddy",
		AuthType: "oauth",
		IsActive: true,
		Data:     model.ConnectionData{AccessToken: "tok"},
	}
	if err := database.CreateConnection(&conn); err != nil {
		t.Fatalf("create: %v", err)
	}

	good := usage.QuotaResult{Quotas: map[string]usage.Quota{
		"pack": {Used: 1, Total: 100, Remaining: 99, ResetAt: "2026-12-01T00:00:00Z"},
	}}
	srv.persistQuotaDeadline(&conn, good)

	stored, err := database.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	first, _ := stored.Data.ProviderSpecificData["quotaDeadline"].(string)
	if first == "" {
		t.Fatal("first successful fetch must store a deadline")
	}

	// Now simulate the quota API failing.
	failed := usage.QuotaResult{Message: "codebuddy usage request failed: 500"}
	srv.persistQuotaDeadline(&conn, failed)

	stored, err = database.GetConnection(conn.ID)
	if err != nil {
		t.Fatalf("reload after failure: %v", err)
	}
	got, _ := stored.Data.ProviderSpecificData["quotaDeadline"].(string)
	if got != first {
		t.Fatalf("failed fetch must keep the last known deadline: had %q, now %q", first, got)
	}
}
