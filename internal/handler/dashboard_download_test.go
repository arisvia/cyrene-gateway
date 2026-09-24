package handler

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arisvia/cyrene-gateway/internal/config"
)

func panelArchive(t *testing.T, entries [][2]string) []byte {
	t.Helper()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	for _, entry := range entries {
		f, err := zw.Create(entry[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(entry[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestPanelExtractionPreservesLastGoodVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries [][2]string
	}{
		{"unsafe entry after index", [][2]string{{"index.html", "new"}, {"../escape", "bad"}}},
		{"oversized asset", [][2]string{{"index.html", "new"}, {"assets/app.js", strings.Repeat("x", maxPanelFileSize+1)}}},
		{"missing index", [][2]string{{"assets/app.js", "new"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := NewDashboardHandler(&config.Config{DataDir: t.TempDir()})
			if err := d.extractPanel(panelArchive(t, [][2]string{{"index.html", "original"}, {"assets/app.js", "old-js"}})); err != nil {
				t.Fatal(err)
			}
			if err := d.extractPanel(panelArchive(t, tc.entries)); err == nil {
				t.Fatal("invalid archive was accepted")
			}
			for name, want := range map[string]string{"index.html": "original", "assets/app.js": "old-js"} {
				got, err := os.ReadFile(filepath.Join(d.panelDir(), name))
				if err != nil || string(got) != want {
					t.Fatalf("last good %s lost: %q, %v", name, got, err)
				}
			}
		})
	}
}

func TestPanelExtractionReplacesCompleteVersion(t *testing.T) {
	d := NewDashboardHandler(&config.Config{DataDir: t.TempDir()})
	if err := d.extractPanel(panelArchive(t, [][2]string{{"index.html", "old"}, {"obsolete.js", "old"}})); err != nil {
		t.Fatal(err)
	}
	if err := d.extractPanel(panelArchive(t, [][2]string{{"dist/index.html", "new"}, {"dist/assets/app.js", "new-js"}})); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(d.panelDir(), "assets", "app.js"))
	if err != nil || string(got) != "new-js" {
		t.Fatalf("new asset not published: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(d.panelDir(), "obsolete.js")); !os.IsNotExist(err) {
		t.Fatalf("stale asset remains: %v", err)
	}
}

func TestPanelDownloadRejectsTruncatedHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxPanelDownload+1))
	}))
	defer srv.Close()
	d := NewDashboardHandler(&config.Config{DataDir: t.TempDir(), PanelURL: srv.URL, AllowPrivateNetworks: true})
	if err := d.writeCache([]byte("last good")); err != nil {
		t.Fatal(err)
	}
	d.TryDownload()
	if got := string(d.readCache()); got != "last good" {
		t.Fatalf("oversized HTML replaced good cache, length %d", len(got))
	}
}
