package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

// Secret management lives in secret.go: it is initialized explicitly after
// config load and persisted under the fixed application data directory.

func sign(data []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verify(data []byte, signature string) bool {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// --- API Key Generation (HMAC-signed) ---

// GenerateAPIKey creates an HMAC-signed API key: cg-<random>.<signature>
func GenerateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	payload := hex.EncodeToString(b)
	sig := sign([]byte(payload))
	return "cg-" + payload + "." + sig
}

// VerifyAPIKeySignature checks the HMAC integrity of an API key.
func VerifyAPIKeySignature(key string) bool {
	if !strings.HasPrefix(key, "cg-") {
		return false
	}
	rest := key[3:]
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) != 2 {
		return false
	}
	return verify([]byte(parts[0]), parts[1])
}

// --- Dashboard Session Token ---

type sessionClaims struct {
	Authenticated bool  `json:"auth"`
	ExpiresAt     int64 `json:"exp"`
}

// CreateSessionToken creates an HMAC-signed session token for dashboard auth.
func CreateSessionToken() (string, error) {
	claims := sessionClaims{
		Authenticated: true,
		ExpiresAt:     time.Now().Add(24 * time.Hour).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign([]byte(encoded))
	return encoded + "." + sig, nil
}

// VerifySessionToken validates a session token and checks expiry.
func VerifySessionToken(token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	if !verify([]byte(parts[0]), parts[1]) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	return claims.Authenticated && time.Now().Unix() < claims.ExpiresAt
}
