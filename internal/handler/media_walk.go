package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/media"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
	"log/slog"
)

// mediaCreds builds upstream credentials for one connection. Media accounts
// rotate exactly like chat accounts, so credentials resolve per attempt.
func mediaCreds(conn *model.ProviderConnection) media.Credentials {
	projectID := ""
	if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok {
			projectID = pid
		}
	}
	return media.Credentials{
		APIKey:      conn.Data.APIKey,
		AccessToken: conn.Data.AccessToken,
		ProjectID:   projectID,
	}
}

// attemptMediaRequest executes one media upstream attempt on a single
// account and classifies the outcome exactly like the chat/messages
// pipelines: transport errors and account-scoped statuses (429/quota/5xx)
// are retryable on the next account; request-scoped 4xx and successes are
// terminal. Error state is persisted so cooldowns apply to media too.
func (s *Server) attemptMediaRequest(
	r *http.Request,
	conn *model.ProviderConnection,
	providerID, modelName, endpoint string,
	do func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error),
) (resp *http.Response, retryable bool, status int, errBody []byte) {
	start := time.Now()
	upResp, err := do(r.Context(), conn)
	latencyMs := int(time.Since(start).Milliseconds())
	if latencyMs < 0 {
		latencyMs = 0
	}
	if err != nil {
		if r.Context().Err() != nil {
			// Client hung up: never cool down an account for a cancellation
			// no account caused (parity with chat/messages walks).
			return nil, false, 0, nil
		}
		slog.Error("Media upstream request failed", "provider", providerID, "model", modelName, "error", err)
		provider.ApplyErrorState(conn, http.StatusBadGateway, err.Error())
		s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		s.recordMediaDetail(providerID, modelName, conn.ID, endpoint, "502", latencyMs)
		return nil, true, http.StatusBadGateway, []byte(fmt.Sprintf("upstream request failed: %v", err))
	}

	if upResp.StatusCode < 200 || upResp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(upResp.Body)
		upResp.Body.Close()
		slog.Warn("Media upstream error response", "status", upResp.StatusCode, "provider", providerID, "model", modelName)
		provider.ApplyErrorState(conn, upResp.StatusCode, string(errBody))
		s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
		s.recordMediaDetail(providerID, modelName, conn.ID, endpoint, fmt.Sprintf("%d", upResp.StatusCode), latencyMs)
		if provider.CheckFallbackError(upResp.StatusCode, string(errBody), conn.Data.BackoffLevel).ShouldFallback {
			return nil, true, upResp.StatusCode, errBody
		}
		return nil, false, upResp.StatusCode, errBody
	}

	provider.ResetAccountState(conn)
	s.DB.UpdateConnectionHealth(conn.ID, conn.Data)
	s.recordMediaDetail(providerID, modelName, conn.ID, endpoint, fmt.Sprintf("%d", upResp.StatusCode), latencyMs)
	return upResp, false, 0, nil
}

// walkMediaAccounts retries account-scoped media failures across the
// provider's active pool, mirroring the chat/messages failover contract.
// Returns the successful response, or (nil, status, errBody) when every
// account failed terminally or the pool is exhausted.
func (s *Server) walkMediaAccounts(
	r *http.Request,
	providerID, modelName, endpoint string,
	conns []model.ProviderConnection,
	first *model.ProviderConnection,
	do func(ctx context.Context, conn *model.ProviderConnection) (*http.Response, error),
) (*http.Response, int, []byte) {
	tried := map[string]bool{}
	var lastStatus int
	var lastBody []byte
	for cur := first; cur != nil; {
		tried[cur.ID] = true
		resp, retryable, status, errBody := s.attemptMediaRequest(r, cur, providerID, modelName, endpoint, do)
		if !retryable {
			return resp, status, errBody
		}
		lastStatus, lastBody = status, errBody
		slog.Warn("Media account unavailable, failing over to the next one",
			"provider", providerID, "model", modelName, "connection", cur.ID, "status", status)
		s.publishRequestEvent(r, RequestEvent{Type: "fallback", Provider: providerID, Model: modelName, Status: "fallback", Endpoint: endpoint})
		cur = nil
		for {
			next := s.selectAvailableConnection(conns, "", tried, "")
			if next == nil {
				break
			}
			tried[next.ID] = true
			s.tryRefreshToken(next)
			cur = next
			break
		}
	}
	return nil, lastStatus, lastBody
}

// resolveMediaPool lists active connections for a media provider and picks
// the first eligible account, so handlers can walk the pool on failure.
func (s *Server) resolveMediaPool(providerID string) ([]model.ProviderConnection, *model.ProviderConnection) {
	conns, err := s.DB.ListConnectionsByProvider(providerID)
	if err != nil || len(conns) == 0 {
		return nil, nil
	}
	first := s.selectAvailableConnection(conns, "", nil, "")
	if first == nil {
		return conns, nil
	}
	s.tryRefreshToken(first)
	return conns, first
}

// writeMediaTerminal renders a terminal upstream error or pool exhaustion.
func writeMediaTerminal(w http.ResponseWriter, status int, errBody []byte, providerID string) {
	if status == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("no eligible accounts for provider: %s", providerID),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(wrapErrorWithHint(status, errBody, providerID))
}
