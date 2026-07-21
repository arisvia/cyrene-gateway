package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	raw := hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw + time.Now().String()))
	return "cg-" + hex.EncodeToString(h[:16])
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return fmt.Sprintf("%s...%s", key[:4], key[len(key)-4:])
}
