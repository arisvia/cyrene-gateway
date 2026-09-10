package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// DiscoverAntigravityProject discovers the Cloud AI Companion project ID for an Antigravity account.
func DiscoverAntigravityProject(ctx context.Context, client *http.Client, accessToken string) (string, error) {
	if client == nil {
		client = provider.SafeHTTPClient(15*time.Second, false)
	}

	endpoints := []string{
		provider.AntigravityBaseURL,
		provider.AntigravityDailyURL,
	}

	body := []byte(`{"metadata":{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}}`)

	var lastErr error
	for _, endpoint := range endpoints {
		url := endpoint + "/v1internal:loadCodeAssist"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", provider.AntigravityUserAgent)
		req.Header.Set("X-Goog-Api-Client", provider.AntigravityXGoogClient)
		req.Header.Set("Client-Metadata", provider.AntigravityMetadata)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("loadCodeAssist HTTP %d: %s", resp.StatusCode, string(respBytes))
			continue
		}

		var payload struct {
			CloudAICompanionProject any `json:"cloudaicompanionProject"`
		}
		if err := json.Unmarshal(respBytes, &payload); err != nil {
			lastErr = err
			continue
		}

		if s, ok := payload.CloudAICompanionProject.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), nil
		}
		if m, ok := payload.CloudAICompanionProject.(map[string]any); ok {
			if id, ok := m["id"].(string); ok && strings.TrimSpace(id) != "" {
				return strings.TrimSpace(id), nil
			}
		}
		// If project not returned directly, try onboarding free-tier user
		if onboardedProj := tryOnboardUser(ctx, client, endpoint, accessToken); onboardedProj != "" {
			return onboardedProj, nil
		}
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no project ID found in loadCodeAssist or onboardUser response")
}

// EnsureAntigravityProject retrieves or lazy-discovers and persists the projectId for an Antigravity connection.
func (s *Server) EnsureAntigravityProject(ctx context.Context, conn *model.ProviderConnection, client *http.Client) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("nil connection")
	}
	if conn.Data.ProviderSpecificData != nil {
		if pid, ok := conn.Data.ProviderSpecificData["projectId"].(string); ok && pid != "" {
			return pid, nil
		}
	}
	if conn.Data.AccessToken == "" {
		return "", fmt.Errorf("missing access token")
	}
	if client == nil {
		client = s.getHTTPClient(15 * time.Second)
	}
	discovered, err := DiscoverAntigravityProject(ctx, client, conn.Data.AccessToken)
	if err != nil {
		return "", err
	}
	if conn.Data.ProviderSpecificData == nil {
		conn.Data.ProviderSpecificData = make(map[string]any)
	}
	conn.Data.ProviderSpecificData["projectId"] = discovered
	if s.DB != nil {
		_ = s.DB.UpdateConnection(conn)
	}
	return discovered, nil
}

// fetchAntigravityCatalog fetches the available models list from Cloud Code Assist.
func (s *Server) fetchAntigravityCatalog(ctx context.Context, client *http.Client, token, projectID string) []model.ModelMetadata {
	if token == "" {
		return nil
	}
	if client == nil {
		client = s.getHTTPClient(15 * time.Second)
	}

	// 1. Try /v1internal:models on daily sandbox (9router PROVIDER_MODELS_CONFIG.antigravity)
	modelsEndpoint := "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:models"
	reqM, errM := http.NewRequestWithContext(ctx, "POST", modelsEndpoint, bytes.NewReader([]byte("{}")))
	if errM == nil {
		reqM.Header.Set("Authorization", "Bearer "+token)
		reqM.Header.Set("Content-Type", "application/json")
		if respM, errDo := client.Do(reqM); errDo == nil {
			defer respM.Body.Close()
			if respM.StatusCode == http.StatusOK {
				if b, errR := io.ReadAll(respM.Body); errR == nil {
					var res struct {
						Models []struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						} `json:"models"`
					}
					if errU := json.Unmarshal(b, &res); errU == nil && len(res.Models) > 0 {
						var out []model.ModelMetadata
						for _, m := range res.Models {
							name := m.Name
							if name == "" {
								name = m.ID
							}
							out = append(out, model.ModelMetadata{ID: m.ID, DisplayName: name})
						}
						return out
					}
				}
			}
		}
	}

	// 2. Try /v1internal:fetchAvailableModels
	endpoints := []string{
		provider.AntigravityBaseURL,
		provider.AntigravityDailyURL,
	}

	body := map[string]any{}
	if projectID != "" {
		body["project"] = projectID
	}
	bodyBytes, _ := json.Marshal(body)

	for _, endpoint := range endpoints {
		url := endpoint + "/v1internal:fetchAvailableModels"
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", provider.AntigravityUserAgent)
		req.Header.Set("X-Goog-Api-Client", provider.AntigravityXGoogClient)
		req.Header.Set("Client-Metadata", provider.AntigravityMetadata)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			slog.Warn("fetchAvailableModels failed", slog.String("endpoint", endpoint), slog.Int("status", resp.StatusCode), slog.String("body", string(respBytes)))
			continue
		}

		// 1. Try Models as map (standard response with rich metadata)
		var resultMap struct {
			Models map[string]struct {
				DisplayName      string `json:"displayName"`
				MaxTokens        int    `json:"maxTokens"`
				MaxOutputTokens  int    `json:"maxOutputTokens"`
				SupportsThinking bool   `json:"supportsThinking"`
				SupportsImages   bool   `json:"supportsImages"`
				IsInternal       bool   `json:"isInternal"`
			} `json:"models"`
		}
		if err := json.Unmarshal(respBytes, &resultMap); err == nil && len(resultMap.Models) > 0 {
			var out []model.ModelMetadata
			for id, m := range resultMap.Models {
				// Filter out internal Google test/telemetry endpoints and completion-only tab anchors
				if m.IsInternal || strings.HasPrefix(id, "chat_") || strings.HasPrefix(id, "tab_") {
					continue
				}
				name := m.DisplayName
				if name == "" {
					name = id
				}
				var caps []string
				var mods []string

				// Distinguish dedicated image generation models (e.g. gemini-3.1-flash-image)
				isImageGen := strings.Contains(strings.ToLower(id), "-image")
				if isImageGen {
					caps = append(caps, "image-generation")
					mods = append(mods, "image")
				} else {
					caps = append(caps, "chat", "code")
					if m.SupportsThinking {
						caps = append(caps, "reasoning")
					}
					if m.SupportsImages {
						caps = append(caps, "vision")
					}
					mods = append(mods, "text")
					if m.SupportsImages {
						mods = append(mods, "image")
					}
				}
				family := "gemini"
				if strings.Contains(id, "claude") {
					family = "claude"
				} else if strings.Contains(id, "gpt-oss") {
					family = "gpt-oss"
				}

				out = append(out, model.ModelMetadata{
					ID:            id,
					DisplayName:   name,
					ContextLength: m.MaxTokens,
					MaxOutput:     m.MaxOutputTokens,
					Capabilities:  caps,
					Modalities:    mods,
					Family:        family,
				})
			}
			if len(out) > 0 {
				// 确定性排序（按 ID 升序），防止每次同步顺序跳变
				sort.Slice(out, func(i, j int) bool {
					return out[i].ID < out[j].ID
				})
				return out
			}
		}

		// 2. Try Models as array
		var resultSlice struct {
			Models []struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
			} `json:"models"`
		}
		if err := json.Unmarshal(respBytes, &resultSlice); err == nil && len(resultSlice.Models) > 0 {
			var out []model.ModelMetadata
			for _, m := range resultSlice.Models {
				if strings.HasPrefix(m.ID, "chat_") || strings.HasPrefix(m.ID, "tab_") {
					continue
				}
				name := m.DisplayName
				if name == "" {
					name = m.ID
				}
				out = append(out, model.ModelMetadata{
					ID:          m.ID,
					DisplayName: name,
				})
			}
			if len(out) > 0 {
				return out
			}
		}
	}

	return nil
}

func tryOnboardUser(ctx context.Context, client *http.Client, endpoint, accessToken string) string {
	url := endpoint + "/v1internal:onboardUser"
	body := map[string]any{
		"tierId":   "FREE",
		"metadata": json.RawMessage(provider.AntigravityMetadata),
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", provider.AntigravityUserAgent)
	req.Header.Set("X-Goog-Api-Client", provider.AntigravityXGoogClient)
	req.Header.Set("Client-Metadata", provider.AntigravityMetadata)

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}

	var payload struct {
		Response struct {
			CloudAICompanionProject struct {
				ID string `json:"id"`
			} `json:"cloudaicompanionProject"`
		} `json:"response"`
	}
	if err := json.Unmarshal(respBytes, &payload); err == nil && payload.Response.CloudAICompanionProject.ID != "" {
		return payload.Response.CloudAICompanionProject.ID
	}

	return ""
}
