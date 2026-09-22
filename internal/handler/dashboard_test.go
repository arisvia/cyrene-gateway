package handler

import (
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/arisvia/cyrene-gateway/internal/config"
)

func dashboardFixture() fstest.MapFS {
	files := fstest.MapFS{}
	modtime := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	for name, data := range map[string]string{
		"index.html":            "<!doctype html><title>Cyrene test panel</title>",
		"assets/app.js":         "console.log('original');",
		"assets/style.css":      "body { color: red; }",
		"assets/br-only.js":     "console.log('brotli only');",
		"assets/plain.js":       "console.log('no sidecars');",
		"assets/bad-sidecar.js": "console.log('directory sidecar');",
		"assets/sniff":          "<!DOCTYPE html><title>Sniff the original</title>",
		"assets/index.html":     "<!doctype html><title>Nested entry</title>",
		"logo.svg":              "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>",
		"assets/orphan.js.br":   "orphan sidecar",
		"assets/directory/file": "directory contents",
	} {
		files[name] = &fstest.MapFile{Data: []byte(data), ModTime: modtime}
	}
	for _, name := range []string{"index.html", "assets/app.js", "assets/style.css", "assets/br-only.js", "assets/sniff", "assets/index.html", "logo.svg"} {
		for _, suffix := range []string{".br", ".gz"} {
			if name == "assets/style.css" && suffix == ".br" || name == "assets/br-only.js" && suffix == ".gz" {
				continue
			}
			// Sidecars are opaque to the handler; distinct bytes detect wrong selection.
			files[name+suffix] = &fstest.MapFile{Data: []byte("sidecar" + suffix + ":" + name), ModTime: modtime.Add(time.Hour)}
		}
	}
	files["assets/bad-sidecar.js.br"] = &fstest.MapFile{Mode: fs.ModeDir | 0o755}
	return files
}

func writeDashboardFixture(t *testing.T, dir string, files fstest.MapFS) {
	t.Helper()
	for name, file := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if file.Mode.IsDir() {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, file.Data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(full, file.ModTime, file.ModTime); err != nil {
			t.Fatal(err)
		}
	}
}

func dashboardForTest(t *testing.T, backend string, files fstest.MapFS) *DashboardHandler {
	t.Helper()
	cfg := &config.Config{DataDir: t.TempDir()}
	d := &DashboardHandler{cfg: cfg}
	switch backend {
	case "local":
		cfg.Dashboard = t.TempDir()
		writeDashboardFixture(t, cfg.Dashboard, files)
	case "downloaded":
		dir := filepath.Join(cfg.DataDir, fmt.Sprintf("panel_dist_%s", Version()))
		writeDashboardFixture(t, dir, files)
	case "embedded":
		d.embedded = files
	default:
		t.Fatalf("unknown backend %q", backend)
	}
	return d
}

func TestDashboardEncodingNegotiation(t *testing.T) {
	cases := []struct {
		name     string
		accept   []string
		file     string
		encoding string
		status   int
	}{
		{name: "absent header"},
		{name: "empty header", accept: []string{""}},
		{name: "brotli", accept: []string{"br"}, encoding: "br"},
		{name: "gzip", accept: []string{"gzip"}, encoding: "gzip"},
		{name: "equal weights prefer br", accept: []string{"gzip, br"}, encoding: "br"},
		{name: "disable br", accept: []string{"br;q=0, gzip"}, encoding: "gzip"},
		{name: "disable gzip", accept: []string{"gzip;q=0, br"}, encoding: "br"},
		{name: "disable both", accept: []string{"br;q=0, gzip;q=0"}},
		{name: "false tokens", accept: []string{"zebra, x-br, gzip2, x-gzip, bramble"}},
		{name: "parameter is not a token", accept: []string{"deflate;br=gzip"}},
		{name: "case and whitespace", accept: []string{" GZIP ; Q = 0.8 , BR ; q=0.4 "}, encoding: "gzip"},
		{name: "gzip higher weight", accept: []string{"br;q=0.2, gzip;q=0.9"}, encoding: "gzip"},
		{name: "br higher weight", accept: []string{"gzip;q=0.2, br;q=0.9"}, encoding: "br"},
		{name: "small positive weight", accept: []string{"br;q=0.001"}, encoding: "br"},
		{name: "multiple header lines", accept: []string{"br;q=0.2", "gzip;q=0.8"}, encoding: "gzip"},
		{name: "wildcard", accept: []string{"*"}, encoding: "br"},
		{name: "wildcard cannot override zero", accept: []string{"br;q=0, *;q=0.8"}, encoding: "gzip"},
		{name: "explicit weight overrides wildcard", accept: []string{"br;q=0.1, *;q=0.9"}, encoding: "gzip"},
		{name: "explicit coding overrides wildcard zero", accept: []string{"*;q=0, gzip;q=0.5"}, encoding: "gzip"},
		{name: "wildcard cannot override both zeros", accept: []string{"*;q=1, br;q=0, gzip;q=0"}},
		{name: "identity preferred", accept: []string{"br;q=0.8, gzip;q=0.7, identity;q=1"}},
		{name: "identity lower weight", accept: []string{"identity;q=0.2, br;q=0.5"}, encoding: "br"},
		{name: "identity same weight", accept: []string{"identity, gzip"}, encoding: "gzip"},
		{name: "identity overrides wildcard zero", accept: []string{"*;q=0, identity;q=0.1"}},
		{name: "identity prohibited with br allowed", accept: []string{"identity;q=0, br"}, encoding: "br"},
		{name: "all prohibited", accept: []string{"br;q=0, gzip;q=0, identity;q=0"}, status: http.StatusNotAcceptable},
		{name: "wildcard zero", accept: []string{"*;q=0"}, status: http.StatusNotAcceptable},
		{name: "unsupported and identity prohibited", accept: []string{"deflate, identity;q=0"}, status: http.StatusNotAcceptable},
		{name: "duplicate cannot revive zero", accept: []string{"br;q=0", "BR;q=1, gzip;q=0.5"}, encoding: "gzip"},
		{name: "duplicate zero wins in reverse order", accept: []string{"br;q=1, br;q=0, gzip"}, encoding: "gzip"},
		{name: "invalid quality", accept: []string{"br;q=NaN, gzip;q=2"}},
		{name: "invalid negative quality", accept: []string{"br;q=-1"}},
		{name: "invalid precision", accept: []string{"br;q=0.9999, gzip;q=1.001"}},
		{name: "invalid parameter", accept: []string{"br;level=1, gzip;q=0.5;other=1"}},
		{name: "invalid empty quality", accept: []string{"br;q="}},
		{name: "gzip fallback", accept: []string{"br, gzip;q=0.5"}, file: "assets/style.css", encoding: "gzip"},
		{name: "br fallback", accept: []string{"gzip, br;q=0.5"}, file: "assets/br-only.js", encoding: "br"},
		{name: "original fallback", accept: []string{"br, gzip"}, file: "assets/plain.js"},
		{name: "unavailable coding", accept: []string{"br"}, file: "assets/style.css"},
		{name: "missing variants and identity prohibited", accept: []string{"br, gzip, identity;q=0"}, file: "assets/plain.js", status: http.StatusNotAcceptable},
		{name: "directory is not a sidecar", accept: []string{"br"}, file: "assets/bad-sidecar.js"},
	}
	for _, backend := range []string{"local", "embedded", "downloaded"} {
		t.Run(backend, func(t *testing.T) {
			files := dashboardFixture()
			d := dashboardForTest(t, backend, files)
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					file := tc.file
					if file == "" {
						file = "assets/app.js"
					}
					r := httptest.NewRequest(http.MethodGet, "/"+file, nil)
					r.Header["Accept-Encoding"] = tc.accept
					w := httptest.NewRecorder()
					w.Header().Add("Vary", "Origin")
					w.Header().Add("Vary", "Accept-Language")
					d.ServeHTTP(w, r)
					status := tc.status
					if status == 0 {
						status = http.StatusOK
					}
					if w.Code != status || w.Header().Get("Content-Encoding") != tc.encoding {
						t.Fatalf("got status=%d encoding=%q, want status=%d encoding=%q", w.Code, w.Header().Get("Content-Encoding"), status, tc.encoding)
					}
					if got := w.Header().Values("Vary"); !reflect.DeepEqual(got, []string{"Origin", "Accept-Language", "Accept-Encoding"}) {
						t.Errorf("Vary = %v", got)
					}
					if status != http.StatusOK {
						if w.Header().Get("Cache-Control") != "" {
							t.Error("negotiation error must not be cached as an immutable asset")
						}
						return
					}
					if got, want := w.Header().Get("Content-Type"), mime.TypeByExtension(filepath.Ext(file)); got != want {
						t.Errorf("Content-Type = %q, want original MIME %q", got, want)
					}
					selected := file
					if tc.encoding == "br" {
						selected += ".br"
					} else if tc.encoding == "gzip" {
						selected += ".gz"
					}
					if got, want := w.Header().Get("Last-Modified"), files[selected].ModTime.Format(http.TimeFormat); got != want {
						t.Errorf("Last-Modified = %q, want selected representation's %q", got, want)
					}
					if w.Body.String() != string(files[selected].Data) {
						t.Errorf("wrong representation: %q", w.Body.String())
					}
				})
			}
		})
	}
}

func TestDashboardRoutesAndCaching(t *testing.T) {
	const noCache = "no-cache, no-store, must-revalidate"
	const immutable = "public, max-age=31536000, immutable"
	for _, backend := range []string{"local", "embedded", "downloaded"} {
		t.Run(backend, func(t *testing.T) {
			files := dashboardFixture()
			d := dashboardForTest(t, backend, files)
			for _, tc := range []struct {
				path, file, cache string
			}{
				{"/", "index.html", noCache},
				{"/index.html", "index.html", noCache},
				{"/settings", "index.html", noCache},
				{"/settings/", "index.html", noCache},
				{"/settings/index.html", "index.html", noCache},
				{"/assets/index.html", "assets/index.html", noCache},
				{"/assets/app.js", "assets/app.js", immutable},
				{"/assets/sniff", "assets/sniff", immutable},
				{"/logo.svg", "logo.svg", ""},
				{"/assets/missing.js", "", ""},
				{"/assets/orphan.js", "", ""},
				{"/assets/directory", "", ""},
				{"/assets/../index.html", "", ""},
			} {
				for _, encoding := range []string{"identity", "br", "gzip"} {
					t.Run(tc.path+"/"+encoding, func(t *testing.T) {
						r := httptest.NewRequest(http.MethodGet, tc.path, nil)
						r.Header.Set("Accept-Encoding", encoding)
						w := httptest.NewRecorder()
						d.ServeHTTP(w, r)
						if w.Header().Get("Vary") != "Accept-Encoding" || w.Header().Get("Cache-Control") != tc.cache {
							t.Errorf("unexpected cache headers: %v", w.Header())
						}
						if tc.file == "" {
							if w.Code != http.StatusNotFound || strings.Contains(w.Header().Get("Content-Type"), "text/html") {
								t.Errorf("missing asset should be a non-HTML 404: %d %v", w.Code, w.Header())
							}
							return
						}
						selected, wantEncoding := tc.file, encoding
						switch encoding {
						case "br":
							selected += ".br"
						case "gzip":
							selected += ".gz"
						default:
							wantEncoding = ""
						}
						if w.Code != http.StatusOK || w.Body.String() != string(files[selected].Data) || w.Header().Get("Content-Encoding") != wantEncoding {
							t.Fatalf("wrong SPA/asset response: %d %v %q", w.Code, w.Header(), w.Body.String())
						}
						wantType := mime.TypeByExtension(filepath.Ext(tc.file))
						if wantType == "" {
							wantType = http.DetectContentType(files[tc.file].Data)
						}
						if w.Header().Get("Content-Type") != wantType {
							t.Errorf("Content-Type = %q, want %q", w.Header().Get("Content-Type"), wantType)
						}
						if r.URL.Path != tc.path {
							t.Errorf("handler changed request path to %q", r.URL.Path)
						}
					})
				}
			}
		})
	}
}

func TestDashboardHTTPFileSemantics(t *testing.T) {
	for _, backend := range []string{"local", "embedded", "downloaded"} {
		t.Run(backend, func(t *testing.T) {
			d := dashboardForTest(t, backend, dashboardFixture())
			for _, path := range []string{"/assets/app.js", "/", "/index.html", "/settings/"} {
				for _, encoding := range []string{"identity", "br", "gzip"} {
					t.Run(path+"/"+encoding, func(t *testing.T) {
						request := func(method string, headers http.Header) *httptest.ResponseRecorder {
							r := httptest.NewRequest(method, path, nil)
							r.Header = headers.Clone()
							if r.Header == nil {
								r.Header = make(http.Header)
							}
							r.Header.Set("Accept-Encoding", encoding)
							w := httptest.NewRecorder()
							d.ServeHTTP(w, r)
							return w
						}
						get := request(http.MethodGet, nil)
						modified := get.Header().Get("Last-Modified")
						if get.Code != http.StatusOK || modified == "" {
							t.Fatalf("missing file metadata: %d %v", get.Code, get.Header())
						}
						head := request(http.MethodHead, nil)
						if head.Code != http.StatusOK || head.Body.Len() != 0 || !reflect.DeepEqual(head.Header(), get.Header()) {
							t.Errorf("HEAD differs from GET metadata or has a body: %d %v", head.Code, head.Header())
						}
						for _, tc := range []struct {
							name    string
							headers http.Header
							status  int
						}{
							{"not modified", http.Header{"If-Modified-Since": {modified}}, http.StatusNotModified},
							{"exists", http.Header{"If-None-Match": {"*"}}, http.StatusNotModified},
							{"etag mismatch", http.Header{"If-Match": {`"missing"`}}, http.StatusPreconditionFailed},
							{"modified since", http.Header{"If-Unmodified-Since": {"Mon, 01 Jan 2001 00:00:00 GMT"}}, http.StatusPreconditionFailed},
							{"etag precedence", http.Header{"If-None-Match": {`"different"`}, "If-Modified-Since": {modified}}, http.StatusOK},
						} {
							t.Run(tc.name, func(t *testing.T) {
								for _, method := range []string{http.MethodGet, http.MethodHead} {
									w := request(method, tc.headers)
									if w.Code != tc.status || (tc.status != http.StatusOK || method == http.MethodHead) && w.Body.Len() != 0 {
										t.Errorf("%s conditional response: %d %q", method, w.Code, w.Body.String())
									}
									if w.Header().Get("Vary") != "Accept-Encoding" || w.Header().Get("Cache-Control") != get.Header().Get("Cache-Control") {
										t.Errorf("lost cache metadata: %v", w.Header())
									}
								}
							})
						}
						partial := request(http.MethodGet, http.Header{"Range": {"bytes=0-3"}})
						if partial.Code != http.StatusPartialContent || partial.Body.String() != get.Body.String()[:4] || partial.Header().Get("Content-Range") != fmt.Sprintf("bytes 0-3/%d", get.Body.Len()) {
							t.Errorf("bad range response: %d %v %q", partial.Code, partial.Header(), partial.Body.String())
						}
						staleRange := request(http.MethodGet, http.Header{"Range": {"bytes=0-3"}, "If-Range": {"Mon, 01 Jan 2001 00:00:00 GMT"}})
						if staleRange.Code != http.StatusOK || staleRange.Body.String() != get.Body.String() {
							t.Errorf("stale If-Range did not return full representation")
						}
					})
				}
			}
		})
	}
}

// Stat can succeed even when opening a sidecar fails (for example, permissions).
type dashboardUnreadableFS struct {
	fs.FS
	denied string
}

func (f dashboardUnreadableFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(f.FS, name)
}

func (f dashboardUnreadableFS) Open(name string) (fs.File, error) {
	if name == f.denied {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}

func TestDashboardUnreadableSidecarFallback(t *testing.T) {
	files := dashboardFixture()
	d := dashboardForTest(t, "embedded", files)
	d.embedded = dashboardUnreadableFS{FS: files, denied: "assets/app.js.br"}
	for _, tc := range []struct {
		accept, encoding, selected string
		status                     int
	}{
		{"br, gzip;q=0.5", "gzip", "assets/app.js.gz", http.StatusOK},
		{"br", "", "assets/app.js", http.StatusOK},
		{"br, identity;q=0", "", "", http.StatusNotAcceptable},
	} {
		t.Run(tc.accept, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
			r.Header.Set("Accept-Encoding", tc.accept)
			w := httptest.NewRecorder()
			d.ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Content-Encoding") != tc.encoding {
				t.Fatalf("wrong fallback: %d %v", w.Code, w.Header())
			}
			if tc.selected != "" && w.Body.String() != string(files[tc.selected].Data) {
				t.Errorf("wrong fallback body: %q", w.Body.String())
			}
		})
	}
}

func TestDashboardPreservesVary(t *testing.T) {
	d := dashboardForTest(t, "embedded", dashboardFixture())
	for _, values := range [][]string{
		nil,
		{"Origin"},
		{"Origin, accept-encoding"},
		{"Origin", "ACCEPT-ENCODING", "Accept-Language"},
		{"*"},
		{"Origin, *"},
		{"X-Accept-Encoding"},
	} {
		t.Run(strings.Join(values, "/"), func(t *testing.T) {
			for _, accept := range []string{"br", "gzip", "identity", "*;q=0"} {
				w := httptest.NewRecorder()
				w.Header()["Vary"] = append([]string(nil), values...)
				r := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
				r.Header.Set("Accept-Encoding", accept)
				d.ServeHTTP(w, r)
				want := append([]string(nil), values...)
				contains := false
				for _, value := range values {
					for token := range strings.SplitSeq(value, ",") {
						token = strings.TrimSpace(token)
						contains = contains || token == "*" || strings.EqualFold(token, "Accept-Encoding")
					}
				}
				if !contains {
					want = append(want, "Accept-Encoding")
				}
				if !reflect.DeepEqual(w.Header().Values("Vary"), want) {
					t.Errorf("Vary = %v, want %v", w.Header().Values("Vary"), want)
				}
			}
		})
	}
}

func TestDashboardTierFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name, path, want string
		local, dist      bool
		legacy, embedded bool
		status           int
	}{
		{"local first", "/", "local", true, true, true, true, http.StatusOK},
		{"downloaded second", "/", "downloaded", false, true, true, true, http.StatusOK},
		{"downloaded SPA", "/settings", "downloaded", false, true, true, true, http.StatusOK},
		{"legacy third", "/", "legacy", false, false, true, true, http.StatusOK},
		{"legacy direct index", "/index.html", "legacy", false, false, true, true, http.StatusOK},
		{"legacy does not intercept SPA", "/settings", "embedded", false, false, true, true, http.StatusOK},
		{"embedded fallback", "/", "embedded", false, false, false, true, http.StatusOK},
		{"unavailable", "/", "", false, false, false, false, http.StatusInternalServerError},
		{"local asset miss never falls through", "/assets/app.js", "", true, true, true, true, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{DataDir: t.TempDir(), Dashboard: t.TempDir()}
			d := &DashboardHandler{cfg: cfg}
			if tc.legacy {
				if err := d.writeCache([]byte("legacy")); err != nil {
					t.Fatal(err)
				}
			}
			if tc.dist {
				writeDashboardFixture(t, filepath.Join(cfg.DataDir, fmt.Sprintf("panel_dist_%s", Version())), fstest.MapFS{"index.html": {Data: []byte("downloaded"), ModTime: time.Now()}})
			}
			if tc.local {
				writeDashboardFixture(t, cfg.Dashboard, fstest.MapFS{"index.html": {Data: []byte("local"), ModTime: time.Now()}})
			}
			if tc.embedded {
				d.embedded = fstest.MapFS{"index.html": {Data: []byte("embedded")}, "assets/app.js": {Data: []byte("embedded asset")}}
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				r := httptest.NewRequest(method, tc.path, nil)
				w := httptest.NewRecorder()
				d.ServeHTTP(w, r)
				if w.Code != tc.status {
					t.Fatalf("status = %d, want %d", w.Code, tc.status)
				}
				if tc.status == http.StatusOK {
					want := tc.want
					if method == http.MethodHead {
						want = ""
					}
					if w.Body.String() != want || w.Header().Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
						t.Errorf("wrong tier response: %q %v", w.Body.String(), w.Header())
					}
				}
			}
		})
	}
}
