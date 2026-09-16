package updater

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DownloadAndVerify downloads the asset and verifies its SHA-256 checksum against checksums.txt.
func DownloadAndVerify(assetURL, checksumURL, expectedAssetName, targetDir string, client *http.Client) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}

	// 1. Fetch checksums.txt
	reqCs, err := http.NewRequest("GET", checksumURL, nil)
	if err != nil {
		return "", fmt.Errorf("create checksums request: %w", err)
	}
	respCs, err := client.Do(reqCs)
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	defer respCs.Body.Close()

	if respCs.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download checksums: status %d", respCs.StatusCode)
	}

	var expectedHash string
	scanner := bufio.NewScanner(respCs.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)
		if len(parts) >= 2 && strings.TrimPrefix(parts[1], "*") == expectedAssetName {
			expectedHash = strings.ToLower(parts[0])
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}
	if expectedHash == "" {
		return "", fmt.Errorf("checksum for %q not found in checksums.txt", expectedAssetName)
	}

	// 2. Download binary to temp file in targetDir (same filesystem partition)
	tmpFile, err := os.CreateTemp(targetDir, "cyrene-update-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
	}()

	reqBin, err := http.NewRequest("GET", assetURL, nil)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("create asset request: %w", err)
	}
	respBin, err := client.Do(reqBin)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("download asset: %w", err)
	}
	defer respBin.Body.Close()

	if respBin.StatusCode != http.StatusOK {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to download asset: status %d", respBin.StatusCode)
	}

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, respBin.Body); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write downloaded asset: %w", err)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualHash, expectedHash) {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("sha256 mismatch for %s: expected %s, got %s", expectedAssetName, expectedHash, actualHash)
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(tmpPath, 0755)
	}

	return tmpPath, nil
}

// ApplyUpdate replaces targetExe with the verified temp file, keeping targetExe.old as backup.
func ApplyUpdate(tempBinaryPath, targetExe string) error {
	var err error
	if targetExe == "" {
		targetExe, err = os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}
	}
	targetExe, err = filepath.EvalSymlinks(targetExe)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}

	oldPath := targetExe + ".old"
	_ = os.Remove(oldPath)

	// Rename current binary to .old (Windows allows renaming running binary)
	if err := os.Rename(targetExe, oldPath); err != nil {
		return fmt.Errorf("rename running binary to .old: %w", err)
	}

	// Move new binary to targetExe
	if err := os.Rename(tempBinaryPath, targetExe); err != nil {
		// Attempt rollback
		_ = os.Rename(oldPath, targetExe)
		return fmt.Errorf("replace executable: %w", err)
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(targetExe, 0755)
	}

	return nil
}

// Rollback restores the previous executable from targetExe.old.
func Rollback(targetExe string) error {
	var err error
	if targetExe == "" {
		targetExe, err = os.Executable()
		if err != nil {
			return fmt.Errorf("get executable path: %w", err)
		}
	}
	targetExe, err = filepath.EvalSymlinks(targetExe)
	if err != nil {
		return fmt.Errorf("resolve symlink: %w", err)
	}

	oldPath := targetExe + ".old"
	if _, err := os.Stat(oldPath); err != nil {
		return fmt.Errorf("no backup binary found at %s: %w", oldPath, err)
	}

	failedPath := targetExe + ".failed"
	_ = os.Remove(failedPath)
	_ = os.Rename(targetExe, failedPath)

	if err := os.Rename(oldPath, targetExe); err != nil {
		return fmt.Errorf("restore backup executable: %w", err)
	}

	return nil
}

// CleanupOldBinary removes the .old backup binary if present.
func CleanupOldBinary(targetExe string) {
	if targetExe == "" {
		exe, err := os.Executable()
		if err != nil {
			return
		}
		targetExe = exe
	}
	_ = os.Remove(targetExe + ".old")
}
