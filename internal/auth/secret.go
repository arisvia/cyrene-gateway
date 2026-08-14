package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Secret lifecycle: the signing secret is initialized explicitly after config
// load (no init()), lives in the fixed application data directory, and is
// separated from password hashing salts (P0-3 / P1-6).

var secret []byte

// InitSecretManager initializes the signing secret used for session tokens
// and API key signatures. Resolution order:
//  1. explicit override (e.g. CYRENE_SECRET / -secret flag)
//  2. CYRENE_AUTH_SECRET environment variable (multi-instance injection)
//  3. <dataDir>/auth-secret file
//  4. freshly generated secret persisted to <dataDir>/auth-secret
//
// All read/write errors are surfaced: a gateway must not silently start with
// a different secret than the one already on disk (that would invalidate
// sessions and API keys).
func InitSecretManager(dataDir, override string) error {
	if override != "" {
		secret = []byte(override)
		return nil
	}
	if env := os.Getenv("CYRENE_AUTH_SECRET"); env != "" {
		secret = []byte(env)
		return nil
	}
	if dataDir == "" {
		return fmt.Errorf("auth: dataDir is required to initialize the secret")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("auth: create data dir: %w", err)
	}
	path := filepath.Join(dataDir, "auth-secret")
	data, err := os.ReadFile(path)
	if err == nil {
		trimmed := strings.TrimSpace(string(data))
		if len(trimmed) < 32 {
			return fmt.Errorf("auth: secret file %s exists but is too short", path)
		}
		secret = []byte(trimmed)
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("auth: read secret file: %w", err)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("auth: generate secret: %w", err)
	}
	generated := hex.EncodeToString(b)
	if err := os.WriteFile(path, []byte(generated), 0o600); err != nil {
		return fmt.Errorf("auth: write secret file: %w", err)
	}
	secret = []byte(generated)
	return nil
}

// HasSecret reports whether the secret manager has been initialized.
func HasSecret() bool { return len(secret) > 0 }

// randomHex returns n random bytes hex-encoded (used for salts).
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
