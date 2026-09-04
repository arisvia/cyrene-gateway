package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

const (
	antigravityBaseURL     = "https://cloudcode-pa.googleapis.com"
	antigravityDailyURL    = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	antigravityUserAgent   = "antigravity/1.15.8 windows/amd64"
	antigravityXGoogClient = "google-cloud-sdk vscode_cloudshelleditor/0.1"
	antigravityMetadata    = `{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`
)

// DiscoverAntigravityProject discovers the Cloud AI Companion project ID for an Antigravity account.
func DiscoverAntigravityProject(ctx context.Context, client *http.Client, accessToken string) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	endpoints := []string{
		antigravityBaseURL,
		antigravityDailyURL,
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
		req.Header.Set("User-Agent", antigravityUserAgent)
		req.Header.Set("X-Goog-Api-Client", antigravityXGoogClient)
		req.Header.Set("Client-Metadata", antigravityMetadata)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)
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

// fetchAntigravityCatalog fetches the available models list from Cloud Code Assist.
func (s *Server) fetchAntigravityCatalog(ctx context.Context, client *http.Client, token, projectID string) []model.ModelMetadata {
	if token == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
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
		antigravityBaseURL,
		antigravityDailyURL,
	}

	body := map[string]any{
		"metadata": json.RawMessage(antigravityMetadata),
	}
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
		req.Header.Set("User-Agent", antigravityUserAgent)
		req.Header.Set("X-Goog-Api-Client", antigravityXGoogClient)
		req.Header.Set("Client-Metadata", antigravityMetadata)

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != http.StatusOK {
			slog.Warn("fetchAvailableModels failed", slog.String("endpoint", endpoint), slog.Int("status", resp.StatusCode), slog.String("body", string(respBytes)))
			continue
		}

		// 1. Try Models as map (standard response)
		var resultMap struct {
			Models map[string]struct {
				DisplayName string `json:"displayName"`
			} `json:"models"`
		}
		if err := json.Unmarshal(respBytes, &resultMap); err == nil && len(resultMap.Models) > 0 {
			var out []model.ModelMetadata
			for id, m := range resultMap.Models {
				name := m.DisplayName
				if name == "" {
					name = id
				}
				out = append(out, model.ModelMetadata{
					ID:          id,
					DisplayName: name,
				})
			}
			return out
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
				name := m.DisplayName
				if name == "" {
					name = m.ID
				}
				out = append(out, model.ModelMetadata{
					ID:          m.ID,
					DisplayName: name,
				})
			}
			return out
		}
	}

	// 3. Fallback to static registered catalog if all remote calls fail (matches 9router behavior)
	if staticModels := provider.GetRegistryModels("antigravity"); len(staticModels) > 0 {
		var out []model.ModelMetadata
		for _, m := range staticModels {
			out = append(out, model.ModelMetadata{
				ID:          m.ID,
				DisplayName: m.Name,
			})
		}
		return out
	}

	return nil
}

func tryOnboardUser(ctx context.Context, client *http.Client, endpoint, accessToken string) string {
	url := endpoint + "/v1internal:onboardUser"
	body := map[string]any{
		"tierId":   "FREE",
		"metadata": json.RawMessage(antigravityMetadata),
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", antigravityUserAgent)
	req.Header.Set("X-Goog-Api-Client", antigravityXGoogClient)
	req.Header.Set("Client-Metadata", antigravityMetadata)

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