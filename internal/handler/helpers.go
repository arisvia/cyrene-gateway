package handler

import (
	"crypto/rand"
	"encoding/hex"
	"runtime/debug"
	"strings"
)

// version is set via ldflags at build time: -ldflags "-X .../handler.version=v0.3.0"
var version string

// Version returns the build version from ldflags, git tag (via build info), or "dev".
// The leading "v" prefix (from git tags) is stripped so the UI can add its own.
func Version() string {
	v := version
	if v == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				v = info.Main.Version
			}
		}
	}
	if v == "" {
		return "dev"
	}
	return strings.TrimPrefix(v, "v")
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
