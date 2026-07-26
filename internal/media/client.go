package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client handles media provider requests with credential resolution.
type Client struct {
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

// Credentials holds the auth token for a media request.
type Credentials struct {
	APIKey      string
	AccessToken string
}

func (c Credentials) Token() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return c.AccessToken
}

// setAuth applies the correct auth header based on config.
func setAuth(req *http.Request, cfg *ProviderConfig, creds Credentials) {
	token := creds.Token()
	if token == "" || cfg.AuthType == "none" {
		return
	}
	switch cfg.AuthHeader {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+token)
	case "x-api-key":
		req.Header.Set("x-api-key", token)
	case "token":
		req.Header.Set("Authorization", "Token "+token)
	case "key":
		// Gemini-style: append as query param
		q := req.URL.Query()
		q.Set("key", token)
		req.URL.RawQuery = q.Encode()
	case "x-subscription-token":
		req.Header.Set("X-Subscription-Token", token)
	default:
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// EmbeddingRequest is the OpenAI-compatible embedding request.
type EmbeddingRequest struct {
	Model          string `json:"model"`
	Input          any    `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
}

// HandleEmbedding proxies an embedding request to the appropriate provider.
func (c *Client) HandleEmbedding(ctx context.Context, providerID string, body []byte, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindEmbedding)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support embeddings", providerID)
	}

	var req EmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid embedding request: %w", err)
	}
	if req.Input == nil {
		return nil, fmt.Errorf("missing required field: input")
	}

	var targetURL string
	var reqBody []byte

	if cfg.Format == "gemini" {
		// Gemini embedding: POST {baseUrl}/{model}:embedContent?key=...
		targetURL = strings.TrimRight(cfg.BaseURL, "/") + "/" + req.Model + ":embedContent"
		geminiBody := map[string]any{
			"model": "models/" + req.Model,
			"content": map[string]any{
				"parts": []map[string]any{{"text": inputToString(req.Input)}},
			},
		}
		reqBody, _ = json.Marshal(geminiBody)
	} else {
		targetURL = cfg.BaseURL
		reqBody = body
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setAuth(httpReq, cfg, creds)

	slog.Debug("Media embedding request",
		slog.String("provider", providerID),
		slog.String("model", req.Model),
	)

	return c.HTTPClient.Do(httpReq)
}

// ImageRequest is the OpenAI-compatible image generation request.
type ImageRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n,omitempty"`
	Size   string `json:"size,omitempty"`
}

// HandleImageGeneration proxies an image generation request.
func (c *Client) HandleImageGeneration(ctx context.Context, providerID string, body []byte, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindImage)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support image generation", providerID)
	}

	var req ImageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid image request: %w", err)
	}
	if req.Prompt == "" {
		return nil, fmt.Errorf("missing required field: prompt")
	}

	var targetURL string
	var reqBody []byte

	switch cfg.Format {
	case "gemini":
		targetURL = strings.TrimRight(cfg.BaseURL, "/") + "/" + req.Model + ":generateContent"
		geminiBody := map[string]any{
			"contents": []map[string]any{
				{"parts": []map[string]any{{"text": req.Prompt}}},
			},
			"generationConfig": map[string]any{
				"responseModalities": []string{"TEXT", "IMAGE"},
			},
		}
		reqBody, _ = json.Marshal(geminiBody)
	default:
		// OpenAI-compatible
		targetURL = cfg.BaseURL
		reqBody = body
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setAuth(httpReq, cfg, creds)

	slog.Debug("Media image request",
		slog.String("provider", providerID),
		slog.String("model", req.Model),
	)

	return c.HTTPClient.Do(httpReq)
}

// TTSRequest is the text-to-speech request.
type TTSRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

// HandleTTS proxies a text-to-speech request and returns audio bytes.
func (c *Client) HandleTTS(ctx context.Context, providerID string, body []byte, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindTTS)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support TTS", providerID)
	}

	var req TTSRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid TTS request: %w", err)
	}
	if req.Input == "" {
		return nil, fmt.Errorf("missing required field: input")
	}

	var targetURL string
	var reqBody []byte

	switch cfg.Format {
	case "openai":
		targetURL = cfg.BaseURL
		reqBody = body
	case "elevenlabs":
		voiceID := req.Voice
		if voiceID == "" {
			voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel default
		}
		targetURL = cfg.BaseURL + "/" + voiceID
		if req.ResponseFormat != "" && req.ResponseFormat != "json" {
			targetURL += "?output_format=" + req.ResponseFormat
		}
		elevenBody := map[string]any{
			"text":     req.Input,
			"model_id": req.Model,
		}
		reqBody, _ = json.Marshal(elevenBody)
	case "gemini-tts":
		targetURL = strings.TrimRight(cfg.BaseURL, "/") + "/" + req.Model + ":generateContent"
		geminiBody := map[string]any{
			"contents": []map[string]any{
				{"parts": []map[string]any{{"text": req.Input}}},
			},
			"generationConfig": map[string]any{
				"responseModalities": []string{"AUDIO"},
			},
		}
		reqBody, _ = json.Marshal(geminiBody)
	default:
		targetURL = cfg.BaseURL
		reqBody = body
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setAuth(httpReq, cfg, creds)

	slog.Debug("Media TTS request",
		slog.String("provider", providerID),
		slog.String("model", req.Model),
	)

	return c.HTTPClient.Do(httpReq)
}

// HandleSTT proxies a speech-to-text request (multipart form).
func (c *Client) HandleSTT(ctx context.Context, providerID string, r *http.Request, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindSTT)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support STT", providerID)
	}

	// Parse multipart form from incoming request
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing required field: file")
	}
	defer file.Close()

	model := r.FormValue("model")

	switch cfg.Format {
	case "openai":
		return c.sttOpenAICompat(ctx, cfg, file, header, model, r, creds)
	case "deepgram":
		return c.sttDeepgram(ctx, cfg, file, header, model, r, creds)
	case "gemini-stt":
		return c.sttGemini(ctx, cfg, file, header, model, r, creds)
	default:
		return c.sttOpenAICompat(ctx, cfg, file, header, model, r, creds)
	}
}

func (c *Client) sttOpenAICompat(ctx context.Context, cfg *ProviderConfig, file io.Reader, header *multipart.FileHeader, model string, r *http.Request, creds Credentials) (*http.Response, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", header.Filename)
	if err != nil {
		return nil, err
	}
	io.Copy(fw, file)
	w.WriteField("model", model)

	// Forward optional fields
	for _, field := range []string{"language", "prompt", "response_format", "temperature"} {
		if v := r.FormValue(field); v != "" {
			w.WriteField(field, v)
		}
	}
	w.Close()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL, &buf)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	setAuth(httpReq, cfg, creds)

	return c.HTTPClient.Do(httpReq)
}

func (c *Client) sttDeepgram(ctx context.Context, cfg *ProviderConfig, file io.Reader, header *multipart.FileHeader, model string, r *http.Request, creds Credentials) (*http.Response, error) {
	audioData, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(cfg.BaseURL)
	q := u.Query()
	q.Set("model", model)
	q.Set("smart_format", "true")
	q.Set("punctuate", "true")
	if lang := r.FormValue("language"); lang != "" {
		q.Set("language", lang)
	} else {
		q.Set("detect_language", "true")
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(audioData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "audio/wav")
	setAuth(httpReq, cfg, creds)

	return c.HTTPClient.Do(httpReq)
}

func (c *Client) sttGemini(ctx context.Context, cfg *ProviderConfig, file io.Reader, header *multipart.FileHeader, model string, r *http.Request, creds Credentials) (*http.Response, error) {
	audioData, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	b64 := base64.StdEncoding.EncodeToString(audioData)

	promptText := "Generate a transcript of the speech. Return only the transcribed text, no commentary."
	if lang := r.FormValue("language"); lang != "" {
		promptText += " Language: " + lang + "."
	}

	targetURL := strings.TrimRight(cfg.BaseURL, "/") + "/" + model + ":generateContent"
	geminiBody := map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]any{
				{"text": promptText},
				{"inline_data": map[string]string{
					"mime_type": "audio/wav",
					"data":      b64,
				}},
			}},
		},
	}
	reqBody, _ := json.Marshal(geminiBody)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setAuth(httpReq, cfg, creds)

	return c.HTTPClient.Do(httpReq)
}

// VideoRequest is the video generation request.
type VideoRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// HandleVideo proxies a video generation request.
func (c *Client) HandleVideo(ctx context.Context, providerID string, action string, body []byte, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindVideo)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support video generation", providerID)
	}

	targetURL := cfg.BaseURL
	if action != "" {
		targetURL = strings.TrimRight(cfg.BaseURL, "/") + "/" + action
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setAuth(httpReq, cfg, creds)

	slog.Debug("Media video request",
		slog.String("provider", providerID),
		slog.String("action", action),
	)

	return c.HTTPClient.Do(httpReq)
}

// HandleVideoStatus polls a video generation job status.
func (c *Client) HandleVideoStatus(ctx context.Context, providerID, requestID string, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindVideo)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support video generation", providerID)
	}

	targetURL := strings.TrimRight(cfg.BaseURL, "/") + "/" + url.PathEscape(requestID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	setAuth(httpReq, cfg, creds)

	return c.HTTPClient.Do(httpReq)
}

// FetchRequest is the web fetch request.
type FetchRequest struct {
	URL      string `json:"url"`
	Format   string `json:"format,omitempty"`
	MaxChars int    `json:"max_characters,omitempty"`
}

// HandleWebFetch proxies a web fetch request.
func (c *Client) HandleWebFetch(ctx context.Context, providerID string, body []byte, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindWebFetch)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support web fetch", providerID)
	}

	var req FetchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid fetch request: %w", err)
	}
	if req.URL == "" {
		return nil, fmt.Errorf("missing required field: url")
	}

	fmt_ := req.Format
	if fmt_ == "" {
		fmt_ = "markdown"
	}

	var targetURL string
	var method string
	var reqBody []byte

	switch cfg.Format {
	case "firecrawl":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{"url": req.URL, "formats": []string{fmt_}})
	case "jina-reader":
		targetURL = cfg.BaseURL + "/" + req.URL
		method = "GET"
	case "tavily":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{"urls": []string{req.URL}, "extract_depth": "basic"})
	case "exa":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{"ids": []string{req.URL}, "text": true})
	default:
		return nil, fmt.Errorf("unsupported fetch provider format: %s", cfg.Format)
	}

	var httpReq *http.Request
	var err error
	if method == "GET" {
		httpReq, err = http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	} else {
		httpReq, err = http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		return nil, err
	}
	setAuth(httpReq, cfg, creds)

	return c.HTTPClient.Do(httpReq)
}

// SearchRequest is the web search request.
type SearchRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

// HandleWebSearch proxies a web search request.
func (c *Client) HandleWebSearch(ctx context.Context, providerID string, body []byte, creds Credentials) (*http.Response, error) {
	cfg := GetConfig(providerID, KindWebSearch)
	if cfg == nil {
		return nil, fmt.Errorf("provider '%s' does not support web search", providerID)
	}

	var req SearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("invalid search request: %w", err)
	}
	if req.Query == "" {
		return nil, fmt.Errorf("missing required field: query")
	}
	if req.MaxResults == 0 {
		req.MaxResults = 5
	}

	var targetURL string
	var method string
	var reqBody []byte

	switch cfg.Format {
	case "brave-search":
		u, _ := url.Parse(cfg.BaseURL)
		q := u.Query()
		q.Set("q", req.Query)
		q.Set("count", fmt.Sprintf("%d", req.MaxResults))
		u.RawQuery = q.Encode()
		targetURL = u.String()
		method = "GET"
	case "tavily":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{
			"query":       req.Query,
			"max_results": req.MaxResults,
		})
	case "exa":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{
			"query":      req.Query,
			"numResults": req.MaxResults,
			"type":       "auto",
		})
	case "serper":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{
			"q":   req.Query,
			"num": req.MaxResults,
		})
	case "searchapi":
		u, _ := url.Parse(cfg.BaseURL)
		q := u.Query()
		q.Set("q", req.Query)
		q.Set("num", fmt.Sprintf("%d", req.MaxResults))
		q.Set("engine", "google")
		u.RawQuery = q.Encode()
		targetURL = u.String()
		method = "GET"
	case "youcom":
		u, _ := url.Parse(cfg.BaseURL)
		q := u.Query()
		q.Set("query", req.Query)
		u.RawQuery = q.Encode()
		targetURL = u.String()
		method = "GET"
	case "linkup":
		targetURL = cfg.BaseURL
		method = "POST"
		reqBody, _ = json.Marshal(map[string]any{
			"q": req.Query,
		})
	case "searxng":
		// SearXNG requires a custom base URL from connection data
		return nil, fmt.Errorf("searxng requires a custom base URL configured in the connection")
	case "google-pse":
		u, _ := url.Parse(cfg.BaseURL)
		q := u.Query()
		q.Set("q", req.Query)
		q.Set("num", fmt.Sprintf("%d", req.MaxResults))
		u.RawQuery = q.Encode()
		targetURL = u.String()
		method = "GET"
	default:
		return nil, fmt.Errorf("unsupported search provider format: %s", cfg.Format)
	}

	var httpReq *http.Request
	var err error
	if method == "GET" {
		httpReq, err = http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	} else {
		httpReq, err = http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(reqBody))
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if err != nil {
		return nil, err
	}
	setAuth(httpReq, cfg, creds)

	return c.HTTPClient.Do(httpReq)
}

func inputToString(input any) string {
	switch v := input.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	b, _ := json.Marshal(input)
	return string(b)
}
