package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/db"
	"github.com/arisvia/cyrene-gateway/internal/model"
	"github.com/arisvia/cyrene-gateway/internal/provider"
)

// The upstream cannot finish until the administrator's edit has been persisted.
func blockedRuntimeUpstream(t *testing.T, status int, body string) (*httptest.Server, <-chan struct{}, func()) {
	t.Helper()
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enteredOnce.Do(func() { close(entered) })
		<-release
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(unblock)
	return upstream, entered, unblock
}

func awaitRuntime(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for runtime operation")
	}
}

func editRuntimeConnection(t *testing.T, database *db.DB, id string, replaceTokens bool) {
	t.Helper()
	conn, err := database.GetConnection(id)
	if err != nil {
		t.Fatal(err)
	}
	conn.IsActive = false
	conn.Name, conn.Email, conn.Priority = "admin name", "admin@example.test", 99
	conn.Data.APIKey, conn.Data.BaseURL = "admin-key", "https://admin.example.test/v1"
	conn.Data.QuotaLimit = 123
	conn.Data.ProviderSpecificData = map[string]any{"proxyPoolId": "admin-pool", "untouched": "admin-value"}
	if replaceTokens {
		conn.Data.AccessToken, conn.Data.RefreshToken = "admin-access", "admin-refresh"
	}
	if err := database.UpdateConnection(conn); err != nil {
		t.Fatal(err)
	}
}

func assertRuntimeConfig(t *testing.T, database *db.DB, id string) *model.ProviderConnection {
	t.Helper()
	conn, err := database.GetConnection(id)
	if err != nil {
		t.Fatal(err)
	}
	if conn.IsActive || conn.Name != "admin name" || conn.Email != "admin@example.test" || conn.Priority != 99 || conn.Data.APIKey != "admin-key" || conn.Data.BaseURL != "https://admin.example.test/v1" || conn.Data.QuotaLimit != 123 || conn.Data.ProviderSpecificData["proxyPoolId"] != "admin-pool" || conn.Data.ProviderSpecificData["untouched"] != "admin-value" {
		t.Errorf("runtime write overwrote administrator configuration: %+v", conn)
	}
	return conn
}

func TestRuntimeHealthPreservesInflightAdminEdit(t *testing.T) {
	for _, mode := range []string{"chat", "combo", "messages", "test"} {
		for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
			t.Run(mode+"/"+http.StatusText(status), func(t *testing.T) {
				srv, database := setupTestServer(t)
				body := `{"id":"response","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
				if status != http.StatusOK {
					body = `{"error":{"message":"rate limit"}}`
				}
				upstream, entered, release := blockedRuntimeUpstream(t, status, body)
				pid := "openai"
				if mode == "messages" {
					pid = "anthropic"
				}
				conn := &model.ProviderConnection{ID: "runtime-health", Provider: pid, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "old-key", BaseURL: upstream.URL, BackoffLevel: 2, LastError: "old error", RateLimitedUntil: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}}
				if err := database.CreateConnection(conn); err != nil {
					t.Fatal(err)
				}
				route, modelID := "/v1/chat/completions", pid+"/gpt-4"
				if mode == "messages" {
					route, modelID = "/v1/messages", "anthropic/claude-sonnet-4"
				}
				if mode == "combo" {
					modelID = "runtime-combo"
					if err := database.CreateCombo(&model.Combo{ID: modelID, Name: modelID, Models: []string{"openai/gpt-4"}}); err != nil {
						t.Fatal(err)
					}
				}
				req := httptest.NewRequest("POST", route, strings.NewReader(`{"model":"`+modelID+`","max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`))
				rec, done := httptest.NewRecorder(), make(chan struct{})
				go func() {
					defer close(done)
					if mode == "test" {
						srv.testConnection(req, conn)
					} else {
						srv.Router.ServeHTTP(rec, req)
					}
				}()
				awaitRuntime(t, entered)
				editRuntimeConnection(t, database, conn.ID, false)
				release()
				awaitRuntime(t, done)
				got := assertRuntimeConfig(t, database, conn.ID)
				if mode != "test" && rec.Code != status {
					t.Errorf("HTTP status = %d, want %d: %s", rec.Code, status, rec.Body.String())
				}
				if status == http.StatusOK {
					if got.Data.LastError != "" {
						t.Errorf("success retained last error: %q", got.Data.LastError)
					}
					if mode != "test" && (got.Data.BackoffLevel != 0 || got.Data.RateLimitedUntil != "") {
						t.Error("success did not clear cooldown/backoff")
					}
				} else if got.Data.LastError == "" {
					t.Error("failure health was not persisted")
				}
			})
		}
	}
}

func TestRuntimeRefreshPreservesInflightAdminEdit(t *testing.T) {
	for _, mode := range []string{"automatic", "manual", "manual-credentials", "manual-deleted", "manual-db-closed"} {
		t.Run(mode, func(t *testing.T) {
			srv, database := setupTestServer(t)
			upstream, entered, release := blockedRuntimeUpstream(t, 200, `{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":3600}`)
			pid := "runtime-refresh-" + mode + "-" + generateID()
			provider.Registry[pid] = provider.ProviderInfo{ID: pid, TokenURL: upstream.URL}
			defer delete(provider.Registry, pid)
			conn := &model.ProviderConnection{ID: pid, Provider: pid, AuthType: "oauth", IsActive: true, Data: model.ConnectionData{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}}
			if err := database.CreateConnection(conn); err != nil {
				t.Fatal(err)
			}
			rec, done := httptest.NewRecorder(), make(chan struct{})
			go func() {
				defer close(done)
				if mode == "automatic" {
					srv.tryRefreshToken(conn)
					return
				}
				req := httptest.NewRequest("POST", "/api/oauth/"+pid+"/refresh", strings.NewReader(`{"connectionId":"`+pid+`"}`))
				srv.Router.ServeHTTP(rec, req)
			}()
			awaitRuntime(t, entered)
			switch mode {
			case "manual-deleted":
				if err := database.DeleteConnection(pid); err != nil {
					t.Fatal(err)
				}
			case "manual-db-closed":
				if err := database.Close(); err != nil {
					t.Fatal(err)
				}
			default:
				editRuntimeConnection(t, database, pid, mode == "manual-credentials")
			}
			release()
			awaitRuntime(t, done)
			if mode == "manual-deleted" || mode == "manual-db-closed" {
				want := http.StatusInternalServerError
				if mode == "manual-deleted" {
					want = http.StatusNotFound
				}
				if rec.Code != want {
					t.Errorf("persistence failure status = %d, want %d: %s", rec.Code, want, rec.Body.String())
				}
				return
			}
			got := assertRuntimeConfig(t, database, pid)
			if mode == "manual-credentials" {
				if got.Data.AccessToken != "admin-access" || got.Data.RefreshToken != "admin-refresh" {
					t.Error("stale refresh replaced administrator credentials")
				}
				if rec.Code != http.StatusConflict {
					t.Errorf("stale refresh status = %d, want 409", rec.Code)
				}
			} else {
				if got.Data.AccessToken != "refreshed-access" || got.Data.RefreshToken != "refreshed-refresh" {
					t.Error("refresh was not persisted")
				}
				if mode == "manual" && rec.Code != http.StatusOK {
					t.Errorf("refresh status = %d: %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestRuntimeProjectDiscoveryPreservesInflightAdminEdit(t *testing.T) {
	srv, database := setupTestServer(t)
	upstream, entered, release := blockedRuntimeUpstream(t, 200, `{"cloudaicompanionProject":"discovered-project"}`)
	conn := &model.ProviderConnection{ID: "runtime-project", Provider: "antigravity", AuthType: "oauth", IsActive: true, Data: model.ConnectionData{AccessToken: "old-access", ProviderSpecificData: map[string]any{"untouched": "old"}}}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}
	// Rewrite only the fixed external discovery destination to a local fixture.
	client := upstream.Client()
	client.Transport = runtimeRewriteTransport{base: client.Transport, destination: upstream.URL}
	done := make(chan struct{})
	var discoveryErr error
	var project string
	go func() {
		defer close(done)
		project, discoveryErr = srv.EnsureAntigravityProject(context.Background(), conn, client)
	}()
	awaitRuntime(t, entered)
	editRuntimeConnection(t, database, conn.ID, false)
	release()
	awaitRuntime(t, done)
	if discoveryErr != nil || project != "discovered-project" {
		t.Fatalf("discovery = %q, %v", project, discoveryErr)
	}
	got := assertRuntimeConfig(t, database, conn.ID)
	if got.Data.ProviderSpecificData["projectId"] != "discovered-project" {
		t.Error("project not persisted")
	}
}

type runtimeRewriteTransport struct {
	base        http.RoundTripper
	destination string
}

func (tr runtimeRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	local, err := http.NewRequestWithContext(req.Context(), req.Method, tr.destination+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	local.Header = req.Header.Clone()
	return tr.base.RoundTrip(local)
}

func TestRuntimeComboNamesAreRoutable(t *testing.T) {
	for _, method := range []string{"POST", "PUT"} {
		for _, name := range []string{"", "   ", "provider/model", "  valid-combo  "} {
			t.Run(method+"/"+name, func(t *testing.T) {
				srv, database := setupTestServer(t)
				route := "/api/combos"
				if method == "PUT" {
					if err := database.CreateCombo(&model.Combo{ID: "combo", Name: "original", Models: []string{"openai/gpt-4"}}); err != nil {
						t.Fatal(err)
					}
					route += "/combo"
				}
				body, err := json.Marshal(map[string]any{"name": name, "models": []string{"openai/gpt-4"}, "kind": "chat"})
				if err != nil {
					t.Fatal(err)
				}
				rec := httptest.NewRecorder()
				srv.Router.ServeHTTP(rec, httptest.NewRequest(method, route, strings.NewReader(string(body))))
				if name != "  valid-combo  " {
					if rec.Code != http.StatusBadRequest {
						t.Errorf("invalid name accepted: %d %s", rec.Code, rec.Body.String())
					}
				} else {
					want := http.StatusOK
					if method == "POST" {
						want = http.StatusCreated
					}
					if rec.Code != want {
						t.Fatalf("valid combo rejected: %d %s", rec.Code, rec.Body.String())
					}
					if _, err := database.GetComboByName("valid-combo"); err != nil {
						t.Errorf("trimmed combo cannot be routed: %v", err)
					}
				}
			})
		}
	}
}

func TestRuntimeModelSyncHonorsBaseURL(t *testing.T) {
	previous := model.GetModelsDevCatalog()
	model.SetModelsDevCatalog(map[string]model.ModelsDevEntry{})
	defer model.SetModelsDevCatalog(previous)
	for _, mode := range []string{"manual", "automatic", "create"} {
		t.Run(mode, func(t *testing.T) {
			srv, database := setupTestServer(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/override/models" {
					_, _ = io.WriteString(w, `{"data":[{"id":"override-model"}]}`)
					return
				}
				_, _ = io.WriteString(w, `{"data":[{"id":"registry-model"}]}`)
			}))
			defer upstream.Close()
			pid := "runtime-models-" + mode
			provider.Registry[pid] = provider.ProviderInfo{ID: pid, BaseURL: upstream.URL, ModelsURL: upstream.URL + "/registry/models", APIType: "openai"}
			defer delete(provider.Registry, pid)
			conn := &model.ProviderConnection{ID: pid, Provider: pid, AuthType: "api-key", IsActive: true, Data: model.ConnectionData{APIKey: "key", BaseURL: upstream.URL + "/override"}}
			if err := database.CreateConnection(conn); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "manual":
				rec := httptest.NewRecorder()
				srv.Router.ServeHTTP(rec, httptest.NewRequest("POST", "/api/providers/"+pid+"/refresh-models", nil))
				if rec.Code != 200 {
					t.Fatalf("manual sync failed: %s", rec.Body.String())
				}
			case "automatic":
				srv.syncAllActiveConnections()
			case "create":
				srv.syncConnectionModels(conn)
			}
			raw, err := database.KVGet("providerModelCache", pid)
			if err != nil {
				t.Fatal(err)
			}
			var cached model.CachedModels
			if err := json.Unmarshal([]byte(raw), &cached); err != nil {
				t.Fatal(err)
			}
			if len(cached.Models) != 1 || cached.Models[0].ID != "override-model" {
				t.Errorf("sync ignored connection base URL: %s", raw)
			}
		})
	}
}

func TestRuntimeResetClearsOnlyHealthAndLocks(t *testing.T) {
	srv, database := setupTestServer(t)
	conn := &model.ProviderConnection{
		ID: "reset-health", Provider: "openai", AuthType: "api-key", IsActive: false,
		Data: model.ConnectionData{
			APIKey: "admin-key", BaseURL: "https://admin.example.test", AccessToken: "access", RefreshToken: "refresh",
			TestStatus: "error", LastError: "rate limit", RateLimitedUntil: "later", BackoffLevel: 4,
			ProviderSpecificData: map[string]any{"modelLock_gpt-4": "later", "proxyPoolId": "admin-pool"},
		},
	}
	if err := database.CreateConnection(conn); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.Router.ServeHTTP(rec, httptest.NewRequest("POST", "/api/providers/reset-health/reset", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reset failed: %d %s", rec.Code, rec.Body.String())
	}
	got, err := database.GetConnection(conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsActive || got.Data.APIKey != "admin-key" || got.Data.BaseURL != "https://admin.example.test" || got.Data.AccessToken != "access" || got.Data.RefreshToken != "refresh" || got.Data.ProviderSpecificData["proxyPoolId"] != "admin-pool" {
		t.Error("reset changed administrator configuration or tokens")
	}
	if got.Data.TestStatus != "active" || got.Data.LastError != "" || got.Data.BackoffLevel != 0 || got.Data.RateLimitedUntil != "" {
		t.Error("reset did not clear health fields")
	}
	if _, exists := got.Data.ProviderSpecificData["modelLock_gpt-4"]; exists {
		t.Error("reset did not remove model lock")
	}
}

func TestRuntimeDirectClientReusesTransport(t *testing.T) {
	srv, _ := setupTestServer(t)
	first, second := srv.getHTTPClient(time.Second), srv.getHTTPClient(2*time.Second)
	if first.Transport != second.Transport {
		t.Error("direct route did not reuse the proxy manager transport cache")
	}
	if first == second || first.Timeout != time.Second || second.Timeout != 2*time.Second {
		t.Error("per-request clients must retain independent timeouts")
	}
}
