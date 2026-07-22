package handler

import (
	"crypto/rand"
	"encoding/hex"
	"runtime/debug"
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
