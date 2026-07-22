package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"time"
)

// version is set via ldflags at build time: -ldflags "-X .../handler.version=v0.3.0"
var version string

// Version returns the build version from ldflags, git tag (via build info), or "dev".
func Version() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return "dev"
}

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
