package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSemVer(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"1.1.0", "1.0.0", 1},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.1.0", 0},
		{"v1.2.0", "1.1.9", 1},
		{"1.10.0", "1.2.0", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.1.0-rc1", "1.1.0", 0},
	}

	for _, tt := range tests {
		got := CompareSemVer(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("CompareSemVer(%q, %q) = %d, expected %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestExpectedAssetName(t *testing.T) {
	name := ExpectedAssetName()
	if name == "" {
		t.Fatalf("ExpectedAssetName returned empty")
	}
}

func TestDownloadVerifyAndApply(t *testing.T) {
	tmpDir := t.TempDir()

	dummyContent := []byte("CYRENE-GATEWAY-BINARY-MOCK-v1.2.0")
	hasher := sha256.New()
	hasher.Write(dummyContent)
	dummyHash := hex.EncodeToString(hasher.Sum(nil))

	assetName := ExpectedAssetName()

	// Mock HTTP server for assets
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(dummyContent)
		case "/checksums":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, "%s  %s\n", dummyHash, assetName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 1. Test DownloadAndVerify
	tmpPath, err := DownloadAndVerify(server.URL+"/binary", server.URL+"/checksums", assetName, tmpDir, server.Client())
	if err != nil {
		t.Fatalf("DownloadAndVerify failed: %v", err)
	}

	// 2. Setup mock target executable
	targetExe := filepath.Join(tmpDir, "cyrene-gateway.exe")
	if err := os.WriteFile(targetExe, []byte("CYRENE-OLD-v1.1.0"), 0755); err != nil {
		t.Fatalf("create mock exe: %v", err)
	}

	// 3. Test ApplyUpdate
	if err := ApplyUpdate(tmpPath, targetExe); err != nil {
		t.Fatalf("ApplyUpdate failed: %v", err)
	}

	// Verify content was updated
	newContent, err := os.ReadFile(targetExe)
	if err != nil {
		t.Fatalf("read updated exe: %v", err)
	}
	if string(newContent) != string(dummyContent) {
		t.Errorf("expected updated content, got %s", string(newContent))
	}

	// Verify backup .old exists
	oldPath := targetExe + ".old"
	oldContent, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("read backup .old: %v", err)
	}
	if string(oldContent) != "CYRENE-OLD-v1.1.0" {
		t.Errorf("expected old content in .old, got %s", string(oldContent))
	}

	// 4. Test Rollback
	if err := Rollback(targetExe); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	rolledBackContent, err := os.ReadFile(targetExe)
	if err != nil {
		t.Fatalf("read rolled back exe: %v", err)
	}
	if string(rolledBackContent) != "CYRENE-OLD-v1.1.0" {
		t.Errorf("expected rolled back content, got %s", string(rolledBackContent))
	}

	// 5. Test CleanupOldBinary
	if err := os.WriteFile(oldPath, []byte("garbage"), 0644); err != nil {
		t.Fatalf("write mock old: %v", err)
	}
	CleanupOldBinary(targetExe)
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected .old to be removed by CleanupOldBinary")
	}
}
