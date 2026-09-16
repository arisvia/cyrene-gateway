package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultGitHubRepo = "arisvia/cyrene-gateway"
	cacheTTL          = 3 * time.Minute
)

// ReleaseAsset represents an asset in a GitHub Release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// GitHubRelease represents GitHub Release payload.
type GitHubRelease struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	PublishedAt string         `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

// UpdateCheckResult is the response returned to API callers.
type UpdateCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseNotes   string `json:"releaseNotes"`
	PublishedAt    string `json:"publishedAt"`
	AssetURL       string `json:"assetUrl,omitempty"`
	AssetName      string `json:"assetName,omitempty"`
	AssetSize      int64  `json:"assetSize,omitempty"`
	ChecksumURL    string `json:"checksumUrl,omitempty"`
}

var (
	cacheMu     sync.Mutex
	cachedCheck *UpdateCheckResult
	cachedAt    time.Time
)

// ExpectedAssetName returns the expected release binary name for current platform.
func ExpectedAssetName() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("cyrene-gateway-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

// CheckForUpdate queries GitHub API for the latest release, comparing against currentVersion.
func CheckForUpdate(currentVersion string, client *http.Client, repo string) (*UpdateCheckResult, error) {
	cacheMu.Lock()
	if cachedCheck != nil && time.Since(cachedAt) < cacheTTL && cachedCheck.CurrentVersion == currentVersion {
		res := *cachedCheck
		cacheMu.Unlock()
		return &res, nil
	}
	cacheMu.Unlock()

	if repo == "" {
		repo = DefaultGitHubRepo
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "cyrene-gateway/"+currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release json: %w", err)
	}

	latestVer := strings.TrimPrefix(rel.TagName, "v")
	cleanCurrent := strings.TrimPrefix(currentVersion, "v")

	hasUpdate := false
	if cleanCurrent != "dev" && cleanCurrent != "" {
		hasUpdate = CompareSemVer(latestVer, cleanCurrent) > 0
	}

	targetAsset := ExpectedAssetName()
	var assetURL string
	var assetSize int64
	var checksumURL string

	for _, a := range rel.Assets {
		if a.Name == targetAsset {
			assetURL = a.BrowserDownloadURL
			assetSize = a.Size
		} else if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
		}
	}

	res := &UpdateCheckResult{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVer,
		HasUpdate:      hasUpdate,
		ReleaseNotes:   rel.Body,
		PublishedAt:    rel.PublishedAt,
		AssetURL:       assetURL,
		AssetName:      targetAsset,
		AssetSize:      assetSize,
		ChecksumURL:    checksumURL,
	}

	cacheMu.Lock()
	cachedCheck = res
	cachedAt = time.Now()
	cacheMu.Unlock()

	return res, nil
}

// CompareSemVer returns 1 if v1 > v2, -1 if v1 < v2, and 0 if v1 == v2.
func CompareSemVer(v1, v2 string) int {
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.SplitN(v1, "-", 2)[0]
	parts2 := strings.SplitN(v2, "-", 2)[0]

	nums1 := strings.Split(parts1, ".")
	nums2 := strings.Split(parts2, ".")

	maxLen := max(len(nums1), len(nums2))

	for i := range maxLen {
		var n1, n2 int
		if i < len(nums1) {
			n1, _ = strconv.Atoi(nums1[i])
		}
		if i < len(nums2) {
			n2, _ = strconv.Atoi(nums2[i])
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	return 0
}
