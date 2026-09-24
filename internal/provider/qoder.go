package provider

// Qoder COSY protocol ported from 9router (shared/qoder/).
//
// Qoder's inference endpoint requires:
//   - Body encoded with a WAF-bypass scheme (base64 → rearrange → custom alphabet)
//   - COSY headers: RSA-wrapped AES key + MD5 signature + machine fingerprint
//   - Response SSE envelope {statusCodeValue, body} unwrapped to plain OpenAI SSE
//
// Auth is a device-token flow: user opens qoder.com/device/selectAccounts with
// a PKCE challenge, then we poll openapi.qoder.sh/api/v1/deviceToken/poll.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- Qoder constants (from shared/qoder/constants.js) ---

const (
	QoderChatURL  = "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	QoderLoginURL = "https://qoder.com/device/selectAccounts"

	QoderCnChatURL  = "https://gateway.qoder.com.cn/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common&Encode=1"
	QoderCnLoginURL = "https://qoder.com.cn/device/selectAccounts"

	qoderIDEVersion   = "1.26.0"
	qoderClientType   = "5"
	qoderDataPolicy   = "disagree"
	qoderLoginVersion = "v2"
	qoderMachineOS    = "x86_64_windows"
	qoderMachineType  = "5"
)

var (
	QoderDeviceTokenURL   = "https://openapi.qoder.sh/api/v1/deviceToken/poll"
	QoderCnDeviceTokenURL = "https://openapi.qoder.com.cn/api/v1/deviceToken/poll"
	QoderUserinfoURL      = "https://openapi.qoder.sh/api/v1/userinfo"
	QoderCnUserinfoURL    = "https://openapi.qoder.com.cn/api/v1/userinfo"
)

// RSA public key for COSY encryption (from Qoder IDE v0.9).
const qoderRSAPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDA8iMH5c02LilrsERw9t6Pv5Nc
4k6Pz1EaDicBMpdpxKduSZu5OANqUq8er4GM95omAGIOPOh+Nx0spthYA2BqGz+l
6HRkPJ7S236FZz73In/KVuLnwI8JJ2CbuJap8kvheCCZpmAWpb/cPx/3Vr/J6I17
XcW+ML9FoCI6AOvOzwIDAQAB
-----END PUBLIC KEY-----`

// --- Body encoding (from shared/qoder/encoding.js) ---

const qoderStdAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
const qoderCustomAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"

var qoderS2C [128]int16

func init() {
	for i := range qoderS2C {
		qoderS2C[i] = -1
	}
	for i := range 64 {
		qoderS2C[qoderStdAlphabet[i]] = int16(qoderCustomAlphabet[i])
	}
	qoderS2C['='] = int16('$')
}

// QoderEncodeBody encodes plaintext using Qoder's WAF-bypass scheme:
// base64 → rearrange [tail][mid][head] → custom alphabet substitution.
func QoderEncodeBody(plaintext []byte) string {
	std := base64.StdEncoding.EncodeToString(plaintext)
	n := len(std)
	a := n / 3
	// [tail][mid][head]
	rearranged := std[n-a:] + std[a:n-a] + std[:a]

	out := make([]byte, n)
	for i := range n {
		c := rearranged[i]
		if c < 128 && qoderS2C[c] >= 0 {
			out[i] = byte(qoderS2C[c])
		} else {
			out[i] = c
		}
	}
	return string(out)
}

// --- COSY signing (from shared/qoder/cosy.js) ---

func qoderAesEncryptCBCBase64(plaintext, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	// PKCS7 padding
	padding := 16 - (len(plaintext) % 16)
	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padding)
	}
	// IV = key bytes (matches JS implementation)
	iv := key[:16]
	cbc := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(padded))
	cbc.CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func qoderRSAEncryptBase64(data []byte, customPEM string) (string, error) {
	pemStr := qoderRSAPublicKeyPEM
	if customPEM != "" {
		pemStr = customPEM
	}
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return "", fmt.Errorf("failed to parse RSA public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse RSA public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("not an RSA public key")
	}
	encrypted, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, data)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func qoderMD5Hex(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

// computeSigPath strips the leading "/algo" prefix from the request path.
func qoderComputeSigPath(requestURL string) string {
	u, err := url.Parse(requestURL)
	if err != nil {
		return ""
	}
	pathname := u.Path
	if strings.HasPrefix(pathname, "/algo") {
		return pathname[len("/algo"):]
	}
	return pathname
}

// QoderCosyCreds holds the credentials needed for COSY signing.
type QoderCosyCreds struct {
	ProviderID   string
	UserID       string
	AuthToken    string
	Name         string
	Email        string
	MachineID    string
	IDEVersion   string
	PublicKeyPEM string
}

// BuildQoderCosyHeaders builds the full Cosy-* header set for a Qoder request.
func BuildQoderCosyHeaders(body []byte, requestURL string, creds QoderCosyCreds) (map[string]string, error) {
	if creds.UserID == "" {
		return nil, fmt.Errorf("cosy: user id is empty")
	}
	if creds.AuthToken == "" {
		return nil, fmt.Errorf("cosy: auth token is empty")
	}

	// Encrypt user info: AES-128-CBC with RSA-wrapped key
	aesKey := []byte(uuid.New().String()[:16])
	userInfo, _ := json.Marshal(map[string]string{
		"uid":                  creds.UserID,
		"security_oauth_token": creds.AuthToken,
		"name":                 creds.Name,
		"aid":                  "",
		"email":                creds.Email,
	})
	infoB64, err := qoderAesEncryptCBCBase64(userInfo, aesKey)
	if err != nil {
		return nil, fmt.Errorf("cosy: AES encrypt failed: %w", err)
	}
	cosyKeyB64, err := qoderRSAEncryptBase64(aesKey, creds.PublicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("cosy: RSA encrypt failed: %w", err)
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	requestID := uuid.New().String()

	cosyVer := qoderIDEVersion
	if creds.IDEVersion != "" {
		cosyVer = creds.IDEVersion
	}
	payloadJSON, _ := json.Marshal(map[string]string{
		"version":     "v1",
		"requestId":   requestID,
		"info":        infoB64,
		"cosyVersion": cosyVer,
		"ideVersion":  "",
	})
	payloadB64 := base64.StdEncoding.EncodeToString(payloadJSON)
	sigPath := qoderComputeSigPath(requestURL)
	sigInput := payloadB64 + "\n" + cosyKeyB64 + "\n" + timestamp + "\n" + string(body) + "\n" + sigPath
	sig := qoderMD5Hex([]byte(sigInput))

	machineID := creds.MachineID
	if machineID == "" {
		machineID = uuid.New().String()
	}
	bodyHash := qoderMD5Hex(body)
	bodyLength := fmt.Sprintf("%d", len(body))

	return map[string]string{
		"Authorization":          "Bearer COSY." + payloadB64 + "." + sig,
		"Cosy-Key":               cosyKeyB64,
		"Cosy-User":              creds.UserID,
		"Cosy-Date":              timestamp,
		"Cosy-Version":           cosyVer,
		"Cosy-Machineid":         machineID,
		"Cosy-Machinetoken":      machineID,
		"Cosy-Machinetype":       qoderMachineType,
		"Cosy-Machineos":         qoderMachineOS,
		"Cosy-Clienttype":        qoderClientType,
		"Cosy-Clientip":          "127.0.0.1",
		"Cosy-Bodyhash":          bodyHash,
		"Cosy-Bodylength":        bodyLength,
		"Cosy-Sigpath":           sigPath,
		"Cosy-Data-Policy":       qoderDataPolicy,
		"Cosy-Organization-Id":   "",
		"Cosy-Organization-Tags": "",
		"Login-Version":          qoderLoginVersion,
		"X-Request-Id":           uuid.New().String(),
	}, nil
}

// --- Qoder device flow (from src/lib/oauth/services/qoder.js) ---

// QoderDeviceFlow holds in-flight device flow state.
type QoderDeviceFlow struct {
	VerificationURI string `json:"verificationUri"`
	CodeVerifier    string `json:"codeVerifier"`
	Nonce           string `json:"nonce"`
	MachineID       string `json:"machineId"`
}

// InitiateQoderDeviceFlow generates PKCE + nonce + machineId and returns
func InitiateQoderDeviceFlow() *QoderDeviceFlow {
	return InitiateQoderDeviceFlowFor("qoder")
}

// InitiateQoderDeviceFlowFor generates PKCE + nonce + machineId for a specific regional provider.
func InitiateQoderDeviceFlowFor(providerID string) *QoderDeviceFlow {
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil
	}

	nonce := uuid.New().String()
	machineID := uuid.New().String()

	params := url.Values{
		"challenge":        {pkce.CodeChallenge},
		"challenge_method": {"S256"},
		"machine_id":       {machineID},
		"nonce":            {nonce},
	}

	loginURL := QoderLoginURL
	if strings.Contains(providerID, "cn") {
		loginURL = QoderCnLoginURL
	}

	return &QoderDeviceFlow{
		VerificationURI: loginURL + "?" + params.Encode(),
		CodeVerifier:    pkce.CodeVerifier,
		Nonce:           nonce,
		MachineID:       machineID,
	}
}

// QoderPollResult is the result of a single device token poll attempt.
type QoderPollResult struct {
	Status       string `json:"status"` // "pending" or "ok"
	AccessToken  string `json:"accessToken,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	UserID       string `json:"userId,omitempty"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"` // unix ms
}

// QoderParseExpiry converts upstream expiry hints to unix-ms.
func QoderParseExpiry(expiresAt any, expiresInSeconds any) int64 {
	now := time.Now().UnixMilli()

	switch v := expiresAt.(type) {
	case float64:
		if v > 0 {
			return int64(v)
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			break
		}
		// Pure numeric string → ms-epoch
		allDigits := true
		for _, c := range trimmed {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(trimmed) > 0 {
			var ms int64
			fmt.Sscanf(trimmed, "%d", &ms)
			if ms > 0 {
				return ms
			}
		}
		if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return t.UnixMilli()
		}
	}

	if v, ok := expiresInSeconds.(float64); ok && v >= 0 {
		return now + int64(v)*1000
	}
	// Default: 30 days
	return now + 30*24*60*60*1000
}

// PollQoderDeviceToken performs a single poll attempt against Qoder's device
// token endpoint. Returns a QoderPollResult with status "pending" or "ok".
func PollQoderDeviceToken(nonce, codeVerifier string, client *http.Client) (*QoderPollResult, error) {
	return PollQoderDeviceTokenFor("qoder", nonce, codeVerifier, client)
}

// PollQoderDeviceTokenFor performs a poll attempt for a specific regional provider.
func PollQoderDeviceTokenFor(providerID, nonce, codeVerifier string, client *http.Client) (*QoderPollResult, error) {
	if nonce == "" || codeVerifier == "" {
		return nil, fmt.Errorf("pollQoderDeviceToken: missing nonce or code verifier")
	}
	if client == nil {
		client = SafeHTTPClient(15*time.Second, false)
	}

	deviceTokenURL := QoderDeviceTokenURL
	if strings.Contains(providerID, "cn") {
		deviceTokenURL = QoderCnDeviceTokenURL
	}

	pollURL := fmt.Sprintf("%s?nonce=%s&verifier=%s&challenge_method=S256",
		deviceTokenURL,
		url.QueryEscape(nonce),
		url.QueryEscape(codeVerifier),
	)
	req, err := http.NewRequest("GET", pollURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Go-http-client/2.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder poll request failed: %w", err)
	}
	defer resp.Body.Close()

	// 202/404 = user hasn't finished the browser flow yet
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNotFound {
		return &QoderPollResult{Status: "pending"}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Message string `json:"message"`
		}
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
			msg = errResp.Message
		}
		return nil, fmt.Errorf("qoder device token poll failed: %s", msg)
	}

	var tokenResp struct {
		ExpiresAt    any    `json:"expires_at"`
		ExpiresIn    any    `json:"expires_in"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("qoder poll: invalid JSON response: %w", err)
	}
	if tokenResp.Token == "" {
		return nil, fmt.Errorf("qoder device token poll returned 200 but no token")
	}

	return &QoderPollResult{
		Status:       "ok",
		AccessToken:  tokenResp.Token,
		RefreshToken: tokenResp.RefreshToken,
		UserID:       tokenResp.UserID,
		ExpiresAt:    QoderParseExpiry(tokenResp.ExpiresAt, tokenResp.ExpiresIn),
	}, nil
}

// QoderUserProfile contains rich profile information for a Qoder user account.
type QoderUserProfile struct {
	Name             string `json:"name"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	SecurityMobile   string `json:"security_mobile"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
}

// EffectiveEmail returns the best account identifier: email if present, else mobile, else username.
func (p *QoderUserProfile) EffectiveEmail() string {
	if p == nil {
		return ""
	}
	if strings.TrimSpace(p.Email) != "" {
		return strings.TrimSpace(p.Email)
	}
	if strings.TrimSpace(p.SecurityMobile) != "" {
		return strings.TrimSpace(p.SecurityMobile)
	}
	return strings.TrimSpace(p.Username)
}

// EffectiveName returns the best display name: name if present, else username.
func (p *QoderUserProfile) EffectiveName() string {
	if p == nil {
		return ""
	}
	if strings.TrimSpace(p.Name) != "" {
		return strings.TrimSpace(p.Name)
	}
	return strings.TrimSpace(p.Username)
}

// FetchQoderUserProfileFor fetches full profile info for a specific regional provider.
func FetchQoderUserProfileFor(providerID, accessToken string, client *http.Client) *QoderUserProfile {
	if strings.TrimSpace(accessToken) == "" {
		return nil
	}
	if client == nil {
		client = SafeHTTPClient(15*time.Second, false)
	}
	userInfoURL := QoderUserinfoURL
	if strings.Contains(providerID, "cn") {
		userInfoURL = QoderCnUserinfoURL
	}
	req, err := http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Go-http-client/2.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var info struct {
		Name             string `json:"name"`
		Username         string `json:"username"`
		Email            string `json:"email"`
		SecurityMobile   string `json:"security_mobile"`
		Mobile           string `json:"mobile"`
		Phone            string `json:"phone"`
		OrganizationID   string `json:"organization_id"`
		OrganizationName string `json:"organization_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil
	}
	secMobile := strings.TrimSpace(info.SecurityMobile)
	if secMobile == "" {
		secMobile = strings.TrimSpace(info.Mobile)
	}
	if secMobile == "" {
		secMobile = strings.TrimSpace(info.Phone)
	}
	return &QoderUserProfile{
		Name:             strings.TrimSpace(info.Name),
		Username:         strings.TrimSpace(info.Username),
		Email:            strings.TrimSpace(info.Email),
		SecurityMobile:   secMobile,
		OrganizationID:   strings.TrimSpace(info.OrganizationID),
		OrganizationName: strings.TrimSpace(info.OrganizationName),
	}
}

// FetchQoderUserInfo fetches profile info for a Qoder token (best-effort).
func FetchQoderUserInfo(accessToken string, client *http.Client) (name, email string) {
	return FetchQoderUserInfoFor("qoder", accessToken, client)
}

// FetchQoderUserInfoFor fetches profile info for a specific regional provider.
func FetchQoderUserInfoFor(providerID, accessToken string, client *http.Client) (name, email string) {
	profile := FetchQoderUserProfileFor(providerID, accessToken, client)
	if profile == nil {
		return "", ""
	}
	return profile.EffectiveName(), profile.EffectiveEmail()
}
