package provider

// Qoder PAT (Personal Access Token) support — ported from 9router
// executors/qoder.js + services/qoderModels.js (9router@9c9dd7b1,
// @d433c0b2).
//
// PATs (pt-...) cannot sign COSY requests directly: they are exchanged for a
// short-lived job token (jt-...) plus a userId via openapi.qoder.sh, then the
// job token is used for COSY signing. Job-token traffic must hit api2.qoder.sh
// — api3 rejects jt- with "Login expired" (403); device tokens (dt-...) stay
// on api3.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Qoder PAT/job-token endpoints (9router shared/qoder/constants.js).
// Vars (not consts) so tests can point them at a mock server.
var (
	QoderJobTokenExchangeURL = "https://openapi.qoder.sh/api/v1/jobToken/exchange"
	QoderPATUserInfoURL      = QoderUserinfoURL
)

const (
	QoderChatBaseAlt = "https://api2.qoder.sh" // jt- traffic (api3 rejects with 403)
	QoderChatSigPath = "/api/v2/service/pro/sse/agent_chat_generation"

	qoderPATRefreshBuffer = 5 * time.Minute
	qoderPATDefaultTTL    = 24 * time.Hour
)

// QoderPATPrefix identifies Personal Access Tokens.
const QoderPATPrefix = "pt-"

// QoderModelListURL resolves the model list endpoint for a token: jt- tokens
// must use api2.qoder.sh (api3 rejects them with 403 "Login expired").
func QoderModelListURL(authToken string) string {
	if strings.HasPrefix(authToken, "jt-") {
		return QoderChatBaseAlt + "/algo/api/v2/model/list"
	}
	return "https://api3.qoder.sh/algo/api/v2/model/list"
}

// QoderChatURLForToken resolves the chat endpoint for a token (same api2/api3
// split as the model list).
func QoderChatURLForToken(authToken string) string {
	base := "https://api3.qoder.sh"
	if strings.HasPrefix(authToken, "jt-") {
		base = QoderChatBaseAlt
	}
	return base + "/algo" + QoderChatSigPath + "?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
}

// IsQoderPAT reports whether a token is a Qoder Personal Access Token.
func IsQoderPAT(token string) bool {
	return strings.HasPrefix(token, QoderPATPrefix)
}

// QoderResolvedCredential is the COSY-signable form of a qoder connection.
type QoderResolvedCredential struct {
	AccessToken string
	UserID      string
	ExpiresAt   time.Time
}

type qoderPATCacheEntry struct {
	cred     *QoderResolvedCredential
	expires  time.Time
	sharedMu *sync.Mutex
}

var (
	qoderPATMu    sync.Mutex
	qoderPATCache = make(map[string]*qoderPATCacheEntry)
)

// ResolveQoderCredential converts a qoder token into COSY-signable form:
//   - PAT (pt-...) → exchanged to a job token (jt-...) + userId, cached until
//     near-expiry (9router resolveQoderCredentials)
//   - anything else → passthrough (token as-is, userId from psd)
func ResolveQoderCredential(token, userID string, client *http.Client) (*QoderResolvedCredential, error) {
	if !IsQoderPAT(token) {
		return &QoderResolvedCredential{AccessToken: token, UserID: userID}, nil
	}
	if token == "" {
		return nil, fmt.Errorf("qoder: empty credential")
	}

	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	// Fast path: cached and not near expiry.
	qoderPATMu.Lock()
	if entry, ok := qoderPATCache[token]; ok && time.Until(entry.expires) > qoderPATRefreshBuffer {
		cred := entry.cred
		qoderPATMu.Unlock()
		return cred, nil
	}
	// Serialize concurrent exchanges for the same PAT.
	entry, ok := qoderPATCache[token]
	if !ok || entry.sharedMu == nil {
		entry = &qoderPATCacheEntry{sharedMu: &sync.Mutex{}}
		qoderPATCache[token] = entry
	}
	mu := entry.sharedMu
	qoderPATMu.Unlock()

	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring the per-PAT lock.
	qoderPATMu.Lock()
	if cached, ok := qoderPATCache[token]; ok && cached.cred != nil && time.Until(cached.expires) > qoderPATRefreshBuffer {
		cred := cached.cred
		qoderPATMu.Unlock()
		return cred, nil
	}
	qoderPATMu.Unlock()

	jobToken, expiresAt, err := qoderExchangeJobToken(token, client)
	if err != nil {
		return nil, err
	}
	resolvedUserID := qoderFetchUserID(jobToken, client)

	cred := &QoderResolvedCredential{
		AccessToken: jobToken,
		UserID:      resolvedUserID,
		ExpiresAt:   expiresAt,
	}

	qoderPATMu.Lock()
	qoderPATCache[token] = &qoderPATCacheEntry{cred: cred, expires: expiresAt, sharedMu: mu}
	qoderPATMu.Unlock()

	return cred, nil
}

// qoderExchangeJobToken exchanges a PAT for a short-lived job token. This
// endpoint is plain JSON POST — NOT COSY-signed (9router exchangeJobToken).
func qoderExchangeJobToken(pat string, client *http.Client) (jobToken string, expiresAt time.Time, err error) {
	body, _ := json.Marshal(map[string]string{"personal_token": pat})
	req, err := http.NewRequest("POST", QoderJobTokenExchangeURL, strings.NewReader(string(body)))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "qodercli/1.0.0")
	req.Header.Set("Cosy-Version", qoderIDEVersion)
	req.Header.Set("Cosy-ClientType", qoderClientType)

	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("qoder PAT exchange failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return "", time.Time{}, fmt.Errorf("qoder PAT exchange failed: %d %s", resp.StatusCode, msg)
	}

	var data struct {
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresAt    string `json:"expires_at"`
		ExpiresIn    any    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", time.Time{}, fmt.Errorf("qoder PAT exchange returned invalid JSON")
	}
	if data.Token == "" {
		return "", time.Time{}, fmt.Errorf("qoder PAT exchange returned no job token")
	}

	expiresAt = time.Now().Add(qoderPATDefaultTTL)
	if data.ExpiresAt != "" {
		if t, perr := time.Parse(time.RFC3339, data.ExpiresAt); perr == nil {
			expiresAt = t
		}
	} else if n, ok := data.ExpiresIn.(float64); ok && n > 0 {
		// 9router treats expires_in as milliseconds for this endpoint.
		expiresAt = time.Now().Add(time.Duration(n) * time.Millisecond)
	}
	return data.Token, expiresAt, nil
}

// qoderFetchUserID resolves the userId for a job token (needed for COSY
// signing). Returns "" on any failure — callers fall back to the stored
// userId (9router fetchUserIdForJobToken).
func qoderFetchUserID(jobToken string, client *http.Client) string {
	req, err := http.NewRequest("GET", QoderPATUserInfoURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+jobToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "qodercli/1.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var info struct {
		ID      string `json:"id"`
		UserID  string `json:"userId"`
		UserID2 string `json:"user_id"`
	}
	if json.NewDecoder(resp.Body).Decode(&info) != nil {
		return ""
	}
	if info.ID != "" {
		return info.ID
	}
	if info.UserID != "" {
		return info.UserID
	}
	return info.UserID2
}

// InvalidateQoderPATCache clears the cached job token for a PAT (test hook /
// credential rotation).
func InvalidateQoderPATCache(pat string) {
	qoderPATMu.Lock()
	delete(qoderPATCache, pat)
	qoderPATMu.Unlock()
}
