package mitm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Server is the MITM TLS interception proxy.
type Server struct {
	port        int
	gatewayPort int
	certMgr     *CertManager
	dnsMgr      *DNSManager
	httpServer  *http.Server
	mu          sync.Mutex
	running     bool
	pid         int

	// Traffic log ring buffer
	logMu      sync.Mutex
	trafficLog []TrafficEntry
}

type TrafficEntry struct {
	Time    string `json:"time"`
	Tool    string `json:"tool"`
	Host    string `json:"host"`
	Path    string `json:"path"`
	Model   string `json:"model,omitempty"`
	Action  string `json:"action"` // "intercepted" or "passthrough"
	Status  int    `json:"status"`
	Latency int64  `json:"latencyMs"`
}

func NewServer(port, gatewayPort int, dataDir string) *Server {
	certDir := dataDir + "/mitm"
	return &Server{
		port:        port,
		gatewayPort: gatewayPort,
		certMgr:     NewCertManager(certDir),
		dnsMgr:      NewDNSManager(),
		trafficLog:  make([]TrafficEntry, 0, 200),
	}
}

// Start initializes certs and starts the MITM TLS proxy.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("MITM server already running")
	}

	if err := s.certMgr.EnsureRootCA(); err != nil {
		return fmt.Errorf("failed to initialize Root CA: %w", err)
	}

	tlsConfig := &tls.Config{
		GetCertificate: s.certMgr.GetCertificate,
	}

	ln, err := tls.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.port), tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", s.port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	s.running = true
	s.pid = 1 // In-process, use sentinel

	go func() {
		slog.Info("MITM proxy started", slog.Int("port", s.port))
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("MITM proxy error", "error", err)
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}
	}()

	return nil
}

// Stop gracefully shuts down the MITM proxy and removes DNS entries.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Remove all DNS entries
	if err := s.dnsMgr.RemoveAll(); err != nil {
		slog.Warn("Failed to remove DNS entries on MITM stop", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.httpServer != nil {
		s.httpServer.Shutdown(ctx)
	}

	s.running = false
	s.pid = 0
	slog.Info("MITM proxy stopped")
	return nil
}

// Status returns current MITM status.
func (s *Server) Status() map[string]any {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	return map[string]any{
		"running":    running,
		"port":       s.port,
		"certExists": s.certMgr.CertExists(),
		"certPath":   s.certMgr.RootCACertPath(),
		"dns":        s.dnsMgr.CheckStatus(),
		"tools":      ToolHosts,
	}
}

// CertPEM returns the Root CA certificate PEM for download.
func (s *Server) CertPEM() ([]byte, error) {
	return s.certMgr.RootCACertPEM()
}

// EnableDNS enables DNS interception for a tool.
func (s *Server) EnableDNS(tool string) error {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	if !running {
		return fmt.Errorf("MITM server is not running")
	}
	return s.dnsMgr.AddEntry(tool)
}

// DisableDNS disables DNS interception for a tool.
func (s *Server) DisableDNS(tool string) error {
	return s.dnsMgr.RemoveEntry(tool)
}

// TrafficLog returns recent traffic entries.
func (s *Server) TrafficLog() []TrafficEntry {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	result := make([]TrafficEntry, len(s.trafficLog))
	copy(result, s.trafficLog)
	return result
}

func (s *Server) addTrafficLog(entry TrafficEntry) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	s.trafficLog = append(s.trafficLog, entry)
	if len(s.trafficLog) > 200 {
		s.trafficLog = s.trafficLog[len(s.trafficLog)-200:]
	}
}

// handleRequest is the main MITM request handler.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Health check endpoint
	if r.URL.Path == "/_mitm_health" {
		writeJSONInternal(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	host := r.Host
	tool := GetToolForHost(host)

	// Not a known tool → passthrough
	if tool == "" {
		s.passthrough(w, r, body, host, start)
		return
	}

	// Check if URL matches chat patterns
	patterns := URLPatterns[tool]
	isChat := false
	for _, p := range patterns {
		if strings.Contains(r.URL.RequestURI(), p) {
			isChat = true
			break
		}
	}

	if !isChat {
		s.passthrough(w, r, body, host, start)
		return
	}

	// Extract model from request
	model := extractModel(r.URL.RequestURI(), body)

	// Intercept: forward to local gateway
	s.intercept(w, r, body, tool, model, host, start)
}

// passthrough forwards the request to the real upstream.
func (s *Server) passthrough(w http.ResponseWriter, r *http.Request, body []byte, host string, start time.Time) {
	// Strip port for actual connection
	targetHost := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		targetHost = h
	}

	// Apply host rewrite for rate-limit avoidance
	if rewritten, ok := HostRewrite[targetHost]; ok {
		if strings.Contains(r.URL.RequestURI(), ":generateContent") || strings.Contains(r.URL.RequestURI(), ":streamGenerateContent") {
			targetHost = rewritten
		}
	}

	// Resolve and connect to real upstream via TLS
	url := fmt.Sprintf("https://%s%s", targetHost, r.URL.RequestURI())

	req, err := http.NewRequest(r.Method, url, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	// Copy headers
	for k, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Host", targetHost)
	req.Host = targetHost

	client := &http.Client{
		Timeout: 5 * time.Minute,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		s.addTrafficLog(TrafficEntry{
			Time:    start.Format(time.RFC3339),
			Host:    host,
			Path:    r.URL.Path,
			Action:  "passthrough",
			Status:  502,
			Latency: time.Since(start).Milliseconds(),
		})
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	s.addTrafficLog(TrafficEntry{
		Time:    start.Format(time.RFC3339),
		Host:    host,
		Path:    r.URL.Path,
		Action:  "passthrough",
		Status:  resp.StatusCode,
		Latency: time.Since(start).Milliseconds(),
	})
}

// intercept forwards the request to the local gateway for AI routing.
func (s *Server) intercept(w http.ResponseWriter, r *http.Request, body []byte, tool, model, host string, start time.Time) {
	gatewayURL := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", s.gatewayPort)

	// Parse original body and inject mapped model
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		// Can't parse → passthrough
		s.passthrough(w, r, body, host, start)
		return
	}

	// Override model with the mapped model if we have one
	if model != "" {
		payload["model"] = model
	}

	newBody, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", gatewayURL, bytes.NewReader(newBody))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MITM-Tool", tool)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("MITM intercept failed", "tool", tool, "error", err)
		s.addTrafficLog(TrafficEntry{
			Time:    start.Format(time.RFC3339),
			Tool:    tool,
			Host:    host,
			Path:    r.URL.Path,
			Model:   model,
			Action:  "intercepted",
			Status:  502,
			Latency: time.Since(start).Milliseconds(),
		})
		http.Error(w, "gateway unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Stream response back
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	s.addTrafficLog(TrafficEntry{
		Time:    start.Format(time.RFC3339),
		Tool:    tool,
		Host:    host,
		Path:    r.URL.Path,
		Model:   model,
		Action:  "intercepted",
		Status:  resp.StatusCode,
		Latency: time.Since(start).Milliseconds(),
	})
}

// extractModel attempts to extract the model from URL path or request body.
func extractModel(url string, body []byte) string {
	// Try URL path (Gemini-style: /models/{model}:generateContent)
	if idx := strings.Index(url, "/models/"); idx >= 0 {
		rest := url[idx+len("/models/"):]
		if end := strings.IndexAny(rest, "/:"); end > 0 {
			return rest[:end]
		}
	}

	// Try JSON body
	var payload struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Model != "" {
		return payload.Model
	}

	return ""
}

func writeJSONInternal(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
