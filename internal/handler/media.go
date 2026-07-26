package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/arisvia/cyrene-gateway/internal/media"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// handleImageGeneration handles POST /v1/images/generations
func (s *Server) handleImageGeneration(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	modelInfo, err := provider.ResolveModel(req.Model, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", req.Model)})
		return
	}

	if !media.SupportsKind(modelInfo.Provider, media.KindImage) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support image generation", modelInfo.Provider)})
		return
	}

	conn, creds := s.resolveMediaCredentials(modelInfo.Provider)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider)})
		return
	}

	// Replace model in body with resolved model
	var bodyMap map[string]any
	json.Unmarshal(bodyBytes, &bodyMap)
	bodyMap["model"] = modelInfo.Model
	bodyBytes, _ = json.Marshal(bodyMap)

	resp, err := s.MediaClient.HandleImageGeneration(r.Context(), modelInfo.Provider, bodyBytes, creds)
	if err != nil {
		slog.Error("Image generation failed", "provider", modelInfo.Provider, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleAudioSpeech handles POST /v1/audio/speech (TTS)
func (s *Server) handleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	modelInfo, err := provider.ResolveModel(req.Model, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", req.Model)})
		return
	}

	if !media.SupportsKind(modelInfo.Provider, media.KindTTS) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support TTS", modelInfo.Provider)})
		return
	}

	conn, creds := s.resolveMediaCredentials(modelInfo.Provider)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider)})
		return
	}

	var bodyMap map[string]any
	json.Unmarshal(bodyBytes, &bodyMap)
	bodyMap["model"] = modelInfo.Model
	bodyBytes, _ = json.Marshal(bodyMap)

	resp, err := s.MediaClient.HandleTTS(r.Context(), modelInfo.Provider, bodyBytes, creds)
	if err != nil {
		slog.Error("TTS failed", "provider", modelInfo.Provider, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleAudioTranscriptions handles POST /v1/audio/transcriptions (STT)
func (s *Server) handleAudioTranscriptions(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to parse multipart form"})
		return
	}

	modelStr := r.FormValue("model")
	if modelStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing model"})
		return
	}

	modelInfo, err := provider.ResolveModel(modelStr, s.DB)
	if err != nil || modelInfo.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("cannot resolve model: %s", modelStr)})
		return
	}

	if !media.SupportsKind(modelInfo.Provider, media.KindSTT) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support STT", modelInfo.Provider)})
		return
	}

	conn, creds := s.resolveMediaCredentials(modelInfo.Provider)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", modelInfo.Provider)})
		return
	}

	resp, err := s.MediaClient.HandleSTT(r.Context(), modelInfo.Provider, r, creds)
	if err != nil {
		slog.Error("STT failed", "provider", modelInfo.Provider, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleVideoGenerations handles POST /v1/videos/generations
func (s *Server) handleVideoGenerations(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Model    string `json:"model"`
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	providerID := req.Provider
	if providerID == "" && req.Model != "" {
		modelInfo, err := provider.ResolveModel(req.Model, s.DB)
		if err == nil && modelInfo.Provider != "" {
			providerID = modelInfo.Provider
		}
	}
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider or model"})
		return
	}

	if !media.SupportsKind(providerID, media.KindVideo) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support video generation", providerID)})
		return
	}

	conn, creds := s.resolveMediaCredentials(providerID)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", providerID)})
		return
	}

	resp, err := s.MediaClient.HandleVideo(r.Context(), providerID, "generations", bodyBytes, creds)
	if err != nil {
		slog.Error("Video generation failed", "provider", providerID, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleVideoStatus handles GET /v1/videos/{id}
func (s *Server) handleVideoStatus(w http.ResponseWriter, r *http.Request) {
	requestID := r.PathValue("id")
	providerID := r.URL.Query().Get("provider")
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider query param"})
		return
	}

	conn, creds := s.resolveMediaCredentials(providerID)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", providerID)})
		return
	}

	resp, err := s.MediaClient.HandleVideoStatus(r.Context(), providerID, requestID, creds)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleWebFetch handles POST /v1/web/fetch
func (s *Server) handleWebFetch(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider"})
		return
	}

	if !media.SupportsKind(req.Provider, media.KindWebFetch) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support web fetch", req.Provider)})
		return
	}

	conn, creds := s.resolveMediaCredentials(req.Provider)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", req.Provider)})
		return
	}

	resp, err := s.MediaClient.HandleWebFetch(r.Context(), req.Provider, bodyBytes, creds)
	if err != nil {
		slog.Error("Web fetch failed", "provider", req.Provider, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleWebSearch handles POST /v1/search
func (s *Server) handleWebSearch(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read request body"})
		return
	}

	var req struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.Provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider"})
		return
	}

	if !media.SupportsKind(req.Provider, media.KindWebSearch) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("provider '%s' does not support web search", req.Provider)})
		return
	}

	conn, creds := s.resolveMediaCredentials(req.Provider)
	if conn == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("no active credentials for provider: %s", req.Provider)})
		return
	}

	resp, err := s.MediaClient.HandleWebSearch(r.Context(), req.Provider, bodyBytes, creds)
	if err != nil {
		slog.Error("Web search failed", "provider", req.Provider, "error", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	s.proxyMediaResponse(w, resp)
}

// handleMediaProviders handles GET /api/media-providers
func (s *Server) handleMediaProviders(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != "" {
		providers := media.GetProvidersByKind(media.Kind(kind))
		writeJSON(w, http.StatusOK, map[string]any{"providers": providers, "kind": kind, "count": len(providers)})
		return
	}

	// Return all grouped by kind
	kinds := []media.Kind{media.KindEmbedding, media.KindImage, media.KindTTS, media.KindSTT, media.KindVideo, media.KindWebFetch, media.KindWebSearch}
	grouped := make(map[string]any)
	for _, k := range kinds {
		grouped[string(k)] = media.GetProvidersByKind(k)
	}
	writeJSON(w, http.StatusOK, map[string]any{"kinds": grouped})
}

// handleMediaVoices handles GET /api/media-providers/tts/voices
func (s *Server) handleMediaVoices(w http.ResponseWriter, r *http.Request) {
	providerID := r.URL.Query().Get("provider")
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing provider query param"})
		return
	}

	// Return static voice lists for known providers
	voices := getStaticVoices(providerID)
	writeJSON(w, http.StatusOK, map[string]any{"provider": providerID, "voices": voices})
}

// resolveMediaCredentials finds the best connection for a media provider and returns credentials.
func (s *Server) resolveMediaCredentials(providerID string) (*model.ProviderConnection, media.Credentials) {
	conns, err := s.DB.ListConnectionsByProvider(providerID)
	if err != nil || len(conns) == 0 {
		return nil, media.Credentials{}
	}

	conn := s.selectAvailableConnection(conns, "", nil)
	if conn == nil {
		return nil, media.Credentials{}
	}

	// Pre-check OAuth token refresh
	s.tryRefreshToken(conn)

	return conn, media.Credentials{
		APIKey:      conn.Data.APIKey,
		AccessToken: conn.Data.AccessToken,
	}
}

// proxyMediaResponse copies an upstream response to the client.
func (s *Server) proxyMediaResponse(w http.ResponseWriter, resp *http.Response) {
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func getStaticVoices(providerID string) []map[string]string {
	switch providerID {
	case "openai":
		return []map[string]string{
			{"id": "alloy", "name": "Alloy"},
			{"id": "echo", "name": "Echo"},
			{"id": "fable", "name": "Fable"},
			{"id": "onyx", "name": "Onyx"},
			{"id": "nova", "name": "Nova"},
			{"id": "shimmer", "name": "Shimmer"},
			{"id": "coral", "name": "Coral"},
			{"id": "verse", "name": "Verse"},
			{"id": "ballad", "name": "Ballad"},
			{"id": "ash", "name": "Ash"},
			{"id": "sage", "name": "Sage"},
			{"id": "amuch", "name": "Amuch"},
			{"id": "dan", "name": "Dan"},
		}
	case "elevenlabs":
		return []map[string]string{
			{"id": "21m00Tcm4TlvDq8ikWAM", "name": "Rachel"},
			{"id": "AZnzlk1XvdvUeBnXmlld", "name": "Domi"},
			{"id": "EXAVITQu4vr4xnSDxMaL", "name": "Bella"},
			{"id": "ErXwobaYiN019PkySvjV", "name": "Antoni"},
			{"id": "MF3mGyEYCl7XYWbV9V6O", "name": "Elli"},
			{"id": "TxGEqnHWrfWFTfGW9XjX", "name": "Josh"},
			{"id": "VR6AewLTigWG4xSOukaG", "name": "Arnold"},
			{"id": "pNInz6obpgDQGcFmaJgB", "name": "Adam"},
			{"id": "yoZ06aMxZJJ28mfd3POQ", "name": "Sam"},
		}
	case "edge-tts":
		return []map[string]string{
			{"id": "en-US-AriaNeural", "name": "Aria (US)"},
			{"id": "en-US-GuyNeural", "name": "Guy (US)"},
			{"id": "en-GB-SoniaNeural", "name": "Sonia (UK)"},
			{"id": "en-GB-RyanNeural", "name": "Ryan (UK)"},
			{"id": "ja-JP-NanamiNeural", "name": "Nanami (JP)"},
			{"id": "zh-CN-XiaoxiaoNeural", "name": "Xiaoxiao (CN)"},
		}
	default:
		return nil
	}
}

// registerMediaRoutes adds media API routes to the server.
func (s *Server) registerMediaRoutes() {
	// OpenAI-compatible media endpoints
	s.Router.HandleFunc("POST /v1/images/generations", s.handleImageGeneration)
	s.Router.HandleFunc("POST /v1/audio/speech", s.handleAudioSpeech)
	s.Router.HandleFunc("POST /v1/audio/transcriptions", s.handleAudioTranscriptions)
	s.Router.HandleFunc("POST /v1/videos/generations", s.handleVideoGenerations)
	s.Router.HandleFunc("GET /v1/videos/{id}", s.handleVideoStatus)
	s.Router.HandleFunc("POST /v1/web/fetch", s.handleWebFetch)
	s.Router.HandleFunc("POST /v1/search", s.handleWebSearch)

	// Dashboard media management API
	s.Router.HandleFunc("GET /api/media-providers", s.handleMediaProviders)
	s.Router.HandleFunc("GET /api/media-providers/tts/voices", s.handleMediaVoices)
}

// mediaProviderID extracts provider from model string (e.g. "openai/dall-e-3" -> "openai")
func mediaProviderID(modelStr string) string {
	if idx := strings.Index(modelStr, "/"); idx > 0 {
		return modelStr[:idx]
	}
	return ""
}
