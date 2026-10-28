package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
						srv.testConnection(req, conn, srv.getHTTPClient(30*time.Second))
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
	local.Host = req.URL.Host
	local.URL.RawQuery = req.URL.RawQuery
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

func TestRuntimeProfileEnrichment(t *testing.T) {
	for _, mode := range []string{"normal", "custom-name", "admin", "config-only", "name", "email", "organization", "legacy-organization", "access-token", "refresh-token", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			srv, database := setupTestServer(t)
			upstream, entered, release := blockedRuntimeUpstream(t, http.StatusOK, `{"name":"Profile Name","email":"profile@example.test","organization_name":"Profile Org","userQuota":{"total":100,"used":10}}`)
			originalURL := provider.QoderCnUserinfoURL
			provider.QoderCnUserinfoURL = upstream.URL + "/profile"
			defer func() { provider.QoderCnUserinfoURL = originalURL }()
			conn := &model.ProviderConnection{
				ID: "profile", Provider: "qoder-cn", AuthType: "oauth", IsActive: true,
				Data: model.ConnectionData{AccessToken: "old-access", RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339), BaseURL: upstream.URL, ProviderSpecificData: map[string]any{"untouched": "old"}},
			}
			if mode == "custom-name" {
				// A custom name matching the newly discovered email is not a generic name.
				conn.Name = "profile@example.test"
			}
			if err := database.CreateConnection(conn); err != nil {
				t.Fatal(err)
			}
			rec, done := httptest.NewRecorder(), make(chan struct{})
			go func() {
				defer close(done)
				req := httptest.NewRequest("GET", "/api/providers/profile/usage", nil)
				req.SetPathValue("id", conn.ID)
				srv.handleConnectionUsage(rec, req)
			}()
			awaitRuntime(t, entered)
			admin, err := database.GetConnection(conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "admin":
				editRuntimeConnection(t, database, conn.ID, true)
			case "deleted":
				if err := database.DeleteConnection(conn.ID); err != nil {
					t.Fatal(err)
				}
			default:
				switch mode {
				case "config-only":
					admin.IsActive, admin.Priority, admin.Data.QuotaLimit = false, 99, 123
					admin.Data.ProviderSpecificData["untouched"] = "admin-value"
				case "name":
					admin.Name = "admin name"
				case "email":
					admin.Email = "admin@example.test"
				case "organization":
					admin.Data.ProviderSpecificData["organizationName"] = "Admin Org"
				case "legacy-organization":
					admin.Data.ProviderSpecificData["organization_name"] = "Admin Org"
				case "access-token":
					admin.Data.AccessToken = "admin-access"
				case "refresh-token":
					admin.Data.RefreshToken = "admin-refresh"
				}
				if err := database.UpdateConnection(admin); err != nil {
					t.Fatal(err)
				}
			}
			release()
			awaitRuntime(t, done)
			if mode == "deleted" {
				if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "profile@example.test") {
					t.Errorf("deleted connection returned old profile: %d %s", rec.Code, rec.Body.String())
				}
				return
			}
			got, err := database.GetConnection(conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "normal" || mode == "custom-name" || mode == "config-only" {
				wantName := "Profile Name"
				if mode == "custom-name" {
					wantName = "profile@example.test"
				}
				if rec.Code != http.StatusOK || got.Name != wantName || got.Email != "profile@example.test" || got.Data.ProviderSpecificData["organizationName"] != "Profile Org" {
					t.Errorf("profile enrichment failed: status=%d connection=%+v", rec.Code, got)
				}
				if mode == "config-only" && (got.IsActive || got.Priority != 99 || got.Data.QuotaLimit != 123 || got.Data.ProviderSpecificData["untouched"] != "admin-value") {
					t.Error("profile enrichment overwrote unrelated administrator configuration")
				}
				return
			}
			if mode == "admin" {
				got = assertRuntimeConfig(t, database, conn.ID)
				if got.Data.AccessToken != "admin-access" || got.Data.RefreshToken != "admin-refresh" {
					t.Error("profile enrichment overwrote replacement credentials")
				}
			} else if got.Name != admin.Name || got.Email != admin.Email || got.Data.AccessToken != admin.Data.AccessToken || got.Data.RefreshToken != admin.Data.RefreshToken || got.Data.ProviderSpecificData["organizationName"] != admin.Data.ProviderSpecificData["organizationName"] || got.Data.ProviderSpecificData["organization_name"] != admin.Data.ProviderSpecificData["organization_name"] {
				t.Errorf("stale enrichment changed administrator profile or credentials: %+v", got)
			}
			if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), "profile@example.test") || strings.Contains(rec.Body.String(), "Profile Org") {
				t.Errorf("stale profile must not be returned: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestConnectionSuccessPersistsModelLockRemoval(t *testing.T) {
	for _, pid := range []string{"openai", "opencode", "opencode-go"} {
		t.Run(pid, func(t *testing.T) {
			srv, database := setupTestServer(t)
			upstream, entered, release := blockedRuntimeUpstream(t, http.StatusOK, `{}`)
			conn := &model.ProviderConnection{
				ID: "locked", Provider: pid, AuthType: "api-key", IsActive: true,
				Data: model.ConnectionData{APIKey: "key", BaseURL: upstream.URL, ProviderSpecificData: map[string]any{
					"modelLock_gpt-4": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
					"modelLock_other": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "proxyPoolId": "old-pool",
				}},
			}
			if err := database.CreateConnection(conn); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			var result testResult
			go func() {
				defer close(done)
				result = srv.testConnection(httptest.NewRequest("POST", "/test", nil), conn, srv.getHTTPClient(30*time.Second))
			}()
			awaitRuntime(t, entered)
			if err := database.UpdateConnectionProviderData(conn.ID, map[string]any{"proxyPoolId": "admin-pool", "extra": "keep"}); err != nil {
				t.Fatal(err)
			}
			release()
			awaitRuntime(t, done)
			if !result.OK {
				t.Fatalf("connection test failed: %+v", result)
			}
			got, err := database.GetConnection(conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"modelLock_gpt-4", "modelLock_other"} {
				if _, exists := got.Data.ProviderSpecificData[key]; exists {
					t.Errorf("successful test left persisted lock %s", key)
				}
			}
			if got.Data.ProviderSpecificData["proxyPoolId"] != "admin-pool" || got.Data.ProviderSpecificData["extra"] != "keep" {
				t.Error("lock removal overwrote unrelated provider configuration")
			}
		})
	}
}

func TestQoderConnectionReportsSigningFailure(t *testing.T) {
	for _, pid := range []string{"qoder", "qoder-cn"} {
		t.Run(pid, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			var exchanges atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/exchange" {
					exchanges.Add(1)
					// Force any repeated resolution to exchange again instead of using the cache.
					_, _ = io.WriteString(w, `{"token":"jt-regional-test","expires_in":1}`)
					return
				}
				cancel()                       // An unsigned model request must not escape to the internet.
				_, _ = io.WriteString(w, `{}`) // Missing user ID cannot be COSY-signed.
			}))
			defer upstream.Close()
			oldExchange, oldCNExchange := provider.QoderJobTokenExchangeURL, provider.QoderCnJobTokenExchangeURL
			oldUser, oldCNUser := provider.QoderPATUserInfoURL, provider.QoderCnPATUserInfoURL
			provider.QoderJobTokenExchangeURL, provider.QoderCnJobTokenExchangeURL = upstream.URL+"/exchange", upstream.URL+"/exchange"
			provider.QoderPATUserInfoURL, provider.QoderCnPATUserInfoURL = upstream.URL+"/userinfo", upstream.URL+"/userinfo"
			defer func() {
				provider.QoderJobTokenExchangeURL, provider.QoderCnJobTokenExchangeURL = oldExchange, oldCNExchange
				provider.QoderPATUserInfoURL, provider.QoderCnPATUserInfoURL = oldUser, oldCNUser
			}()
			pat := "pt-signing-" + generateID()
			defer provider.InvalidateQoderPATCache(pat)
			conn := &model.ProviderConnection{Provider: pid, AuthType: "api-key", Data: model.ConnectionData{APIKey: pat}}
			result := srv.testConnection(httptest.NewRequest("POST", "/test", nil).WithContext(ctx), conn, srv.getHTTPClient(30*time.Second))
			if result.OK || !strings.Contains(result.Error, "cosy: user id is empty") {
				t.Errorf("missing signing error: %+v", result)
			}
			if got := exchanges.Load(); got != 1 {
				t.Errorf("PAT exchanged %d times, want once", got)
			}
		})
	}
}

func TestQoderConnectionUsesRegionalCOSYCredential(t *testing.T) {
	for _, tc := range []struct {
		provider, apiHost, modelHost, token, user string
	}{
		{"qoder", "openapi.qoder.sh", "api2.qoder.sh", "jt-intl-fixture", "intl-user"},
		{"qoder-cn", "openapi.qoder.com.cn", "gateway.qoder.com.cn", "jt-cn-fixture", "cn-user"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			pat := "pt-regional-" + generateID()
			defer provider.InvalidateQoderPATCache(pat)
			var exchanges, models atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				wantHost := tc.apiHost
				if r.URL.Path == "/algo/api/v2/model/list" {
					wantHost = tc.modelHost
				}
				if r.Host != wantHost {
					t.Errorf("request escaped region: host=%q, want %q", r.Host, wantHost)
					http.Error(w, "wrong region", http.StatusBadRequest)
					return
				}
				switch r.URL.Path {
				case "/api/v1/jobToken/exchange":
					exchanges.Add(1)
					var body struct {
						PersonalToken string `json:"personal_token"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Method != http.MethodPost || body.PersonalToken != pat {
						t.Errorf("incorrect PAT exchange: method=%s body=%+v err=%v", r.Method, body, err)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"token": tc.token, "expires_in": 1})
				case "/api/v1/userinfo":
					if r.Header.Get("Authorization") != "Bearer "+tc.token {
						t.Error("profile lookup did not use the regional job token")
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"id": tc.user})
				case "/algo/api/v2/model/list":
					models.Add(1)
					if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer COSY.") || r.Header.Get("Cosy-User") != tc.user || r.Header.Get("Cosy-Key") == "" || r.Header.Get("Cosy-Date") == "" || r.Header.Get("Cosy-Sigpath") != "/api/v2/model/list" || r.Header.Get("Cosy-Bodyhash") != "d41d8cd98f00b204e9800998ecf8427e" {
						t.Errorf("model request lacks regional COSY authentication: %v", r.Header)
						http.Error(w, "missing COSY authentication", http.StatusUnauthorized)
						return
					}
					_, _ = io.WriteString(w, `{"data":[]}`)
				default:
					t.Errorf("unexpected upstream path %q", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			client := upstream.Client()
			client.Transport = runtimeRewriteTransport{base: client.Transport, destination: upstream.URL}
			conn := &model.ProviderConnection{Provider: tc.provider, AuthType: "api-key", Data: model.ConnectionData{APIKey: pat}}
			result := srv.testConnection(httptest.NewRequest("POST", "/test", nil), conn, client)
			if !result.OK || exchanges.Load() != 1 || models.Load() != 1 {
				t.Errorf("regional test result=%+v, exchanges=%d models=%d", result, exchanges.Load(), models.Load())
			}
		})
	}
}

func TestQoderOAuthDeviceTokenConnectionProbesModelList(t *testing.T) {
	for _, tc := range []struct{ provider, modelHost string }{
		{"qoder", "api3.qoder.sh"},
		{"qoder-cn", "gateway.qoder.com.cn"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			srv, _ := setupTestServer(t)
			var models, exchanges atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/jobToken/exchange" {
					exchanges.Add(1)
					http.NotFound(w, r)
					return
				}
				if r.Host != tc.modelHost {
					t.Errorf("request escaped region: host=%q, want %q", r.Host, tc.modelHost)
					http.Error(w, "wrong region", http.StatusBadRequest)
					return
				}
				if r.URL.Path != "/algo/api/v2/model/list" {
					t.Errorf("device-token probe must not derive URLs from the SSE chat base, got %q", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				models.Add(1)
				if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer COSY.") || r.Header.Get("Cosy-User") != "oauth-user" {
					t.Errorf("model request lacks COSY authentication: %v", r.Header)
					http.Error(w, "missing COSY authentication", http.StatusUnauthorized)
					return
				}
				_, _ = io.WriteString(w, `{"data":[]}`)
			}))
			defer upstream.Close()
			client := upstream.Client()
			client.Transport = runtimeRewriteTransport{base: client.Transport, destination: upstream.URL}
			conn := &model.ProviderConnection{
				Provider: tc.provider, AuthType: "oauth",
				Data: model.ConnectionData{
					AccessToken:          "dt-oauth-" + generateID(),
					ProviderSpecificData: map[string]any{"userId": "oauth-user"},
				},
			}
			result := srv.testConnection(httptest.NewRequest("POST", "/test", nil), conn, client)
			if !result.OK || models.Load() != 1 || exchanges.Load() != 0 {
				t.Errorf("device-token test result=%+v, model probes=%d exchanges=%d", result, models.Load(), exchanges.Load())
			}
		})
	}
}

func TestCodebuddyIntlConnectionProbesAccountCatalog(t *testing.T) {
	srv, _ := setupTestServer(t)
	var catalogHits atomic.Int32
	var expectedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected probe path %q, want %q", r.URL.Path, expectedPath)
			http.NotFound(w, r)
			return
		}
		catalogHits.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer jwt-intl-conn" {
			t.Errorf("catalog probe auth header = %q, want bearer access token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"chat":{"free":{"hy3":{}}}}}`)
	}))
	defer upstream.Close()

	orig, ok := provider.Registry["codebuddy"]
	if !ok {
		t.Fatal("codebuddy missing from registry")
	}
	mockInfo := orig
	mockInfo.BaseURL = upstream.URL + "/v2/chat/completions"
	mockInfo.ModelsURL = upstream.URL + "/v3/config"
	provider.Registry["codebuddy"] = mockInfo
	defer func() { provider.Registry["codebuddy"] = orig }()

	t.Run("registry catalog", func(t *testing.T) {
		expectedPath = "/v3/config"
		conn := &model.ProviderConnection{
			Provider: "codebuddy", AuthType: "oauth",
			Data: model.ConnectionData{AccessToken: "jwt-intl-conn"},
		}
		before := catalogHits.Load()
		result := srv.testConnection(httptest.NewRequest("POST", "/test", nil), conn, srv.getHTTPClient(30*time.Second))
		if !result.OK || catalogHits.Load() != before+1 {
			t.Errorf("codebuddy intl test result=%+v, catalog hits=%d", result, catalogHits.Load()-before)
		}
	})

	// A BaseURL override must be probed like model sync derives it, never the
	// public registry catalog with the mirror's token.
	t.Run("base URL override", func(t *testing.T) {
		expectedPath = "/v2/models"
		conn := &model.ProviderConnection{
			Provider: "codebuddy", AuthType: "oauth",
			Data: model.ConnectionData{
				AccessToken: "jwt-intl-conn",
				BaseURL:     upstream.URL + "/v2/chat/completions",
			},
		}
		before := catalogHits.Load()
		result := srv.testConnection(httptest.NewRequest("POST", "/test", nil), conn, srv.getHTTPClient(30*time.Second))
		if !result.OK || catalogHits.Load() != before+1 {
			t.Errorf("override probe result=%+v, hits=%d", result, catalogHits.Load()-before)
		}
	})
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
