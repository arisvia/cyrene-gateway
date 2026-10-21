package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/metrics"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"github.com/arisvia/cyrene-gateway/internal/usage"
)

// recordUsage persists a usage entry to the database and publishes a real-time event.
func (s *Server) recordUsage(uc *usageContext, u usage.Usage) {
	if uc == nil {
		return
	}
	usageKnown := u.Known || u.TotalTokens > 0
	if s.Metrics != nil {
		dur := time.Since(uc.StartedAt).Seconds()
		if dur < 0 {
			dur = 0
		}
		var measuredUsage *metrics.Usage
		if usageKnown {
			measuredUsage = &metrics.Usage{
				PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens,
				CachedTokens: u.CachedTokens, ReasoningTokens: u.ReasoningTokens,
			}
		}
		s.Metrics.ObserveRequest(uc.Provider, uc.Model, uc.Endpoint, uc.Status, dur, measuredUsage)
	}
	if s.DB == nil {
		return
	}
	statusStr := "ok"
	if uc.Status == 499 {
		statusStr = "canceled"
	} else if uc.Status >= 400 {
		statusStr = fmt.Sprintf("%d", uc.Status)
	}

	promptTokens := u.PromptTokens
	completionTokens := u.CompletionTokens
	if !usageKnown {
		if len(uc.Prompt) > 0 {
			promptTokens = len(uc.Prompt) / 4
			if promptTokens < 1 {
				promptTokens = 1
			}
		}
		if len(uc.Response) > 0 {
			completionTokens = len(uc.Response) / 4
			if completionTokens < 1 {
				completionTokens = 1
			}
		}
	}

	entry := &db.UsageEntry{
		Provider:         uc.Provider,
		Model:            uc.Model,
		ConnectionID:     uc.ConnectionID,
		APIKey:           uc.APIKey,
		Endpoint:         uc.Endpoint,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Status:           statusStr,
	}
	meta := map[string]any{}
	if !usageKnown {
		meta["usage_missing"] = true
	}
	if u.CachedTokens > 0 {
		meta["cached_tokens"] = u.CachedTokens
	}
	if u.ReasoningTokens > 0 {
		meta["reasoning_tokens"] = u.ReasoningTokens
	}
	if len(meta) > 0 {
		b, _ := json.Marshal(meta)
		entry.Meta = string(b)
	}
	if err := s.DB.SaveUsageEntry(entry); err != nil {
		slog.Warn("Failed to record usage", "error", err, "model", uc.Model)
	}
	// Persist to requestDetails table for web UI "请求明细"
	latencyMs := int(time.Since(uc.StartedAt).Milliseconds())
	if latencyMs < 0 {
		latencyMs = 0
	}
	rdID := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uc.Model)
	rdData := map[string]any{
		"id":               rdID,
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
		"provider":         uc.Provider,
		"model":            uc.Model,
		"connectionId":     uc.ConnectionID,
		"status":           statusStr,
		"promptTokens":     promptTokens,
		"completionTokens": completionTokens,
		"cost":             entry.Cost,
		"latencyMs":        latencyMs,
		"endpoint":         uc.Endpoint,
		"usageKnown":       usageKnown,
	}
	if uc.Prompt != "" {
		rdData["input"] = uc.Prompt
	}
	if uc.Response != "" {
		rdData["output"] = uc.Response
	}
	if u.CachedTokens > 0 {
		rdData["cachedTokens"] = u.CachedTokens
	}
	if u.ReasoningTokens > 0 {
		rdData["reasoningTokens"] = u.ReasoningTokens
	}
	rdBytes, _ := json.Marshal(rdData)
	if err := s.DB.SaveRequestDetail(&db.RequestDetail{
		ID:           rdID,
		Timestamp:    rdData["timestamp"].(string),
		Provider:     uc.Provider,
		Model:        uc.Model,
		ConnectionID: uc.ConnectionID,
		Status:       statusStr,
		Data:         string(rdBytes),
	}); err != nil {
		slog.Warn("Failed to record request detail", "error", err, "model", uc.Model)
	}

	// Publish real-time event for SSE stream
	if s.Events != nil {
		eventType := "completed"
		if uc.Status >= 400 {
			eventType = "failed"
		}
		s.Events.Publish(RequestEvent{
			RequestID: uc.RequestID,
			Type:      eventType,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Provider:  uc.Provider,
			Model:     uc.Model,
			Status:    statusStr,
			Prompt:    promptTokens,
			Compl:     completionTokens,
			Endpoint:  uc.Endpoint,
			LatencyMs: int64(latencyMs),
		})
	}
}

// selectAvailableConnection picks the best connection using priority, cooldown, model locks, and quota.
func (s *Server) selectAvailableConnection(conns []model.ProviderConnection, modelName string, excludeIDs map[string]bool) *model.ProviderConnection {
	return provider.SelectCredentialWithQuota(conns, modelName, excludeIDs, s.quotaChecker())
}
