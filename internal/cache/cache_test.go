package cache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestComputeKey(t *testing.T) {
	// Two requests with different key order but identical content
	req1 := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"temperature":0}`)
	req2 := []byte(`{"temperature":0,"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)

	k1, err1 := ComputeKey("chat", req1)
	if err1 != nil {
		t.Fatalf("ComputeKey req1 failed: %v", err1)
	}
	k2, err2 := ComputeKey("chat", req2)
	if err2 != nil {
		t.Fatalf("ComputeKey req2 failed: %v", err2)
	}

	if k1 != k2 {
		t.Fatalf("expected identical keys for same content with different key order, got %s != %s", k1, k2)
	}

	// Requests with different extra fields should NOT collide
	req3 := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"temperature":0,"seed":42}`)
	k3, err3 := ComputeKey("chat", req3)
	if err3 != nil {
		t.Fatalf("ComputeKey req3 failed: %v", err3)
	}
	if k1 == k3 {
		t.Fatalf("expected different keys when seed is present, got collision: %s", k1)
	}

	// Strips stream parameter
	reqStream := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"temperature":0,"stream":false}`)
	kStream, errStream := ComputeKey("chat", reqStream)
	if errStream != nil {
		t.Fatalf("ComputeKey reqStream failed: %v", errStream)
	}
	if k1 != kStream {
		t.Fatalf("expected stream param to be stripped during key computation, got %s != %s", k1, kStream)
	}
}

func TestIsEligible(t *testing.T) {
	// Streaming is never eligible
	streamReq := []byte(`{"model":"gpt-4o","messages":[],"stream":true,"temperature":0}`)
	eligible, _, _ := IsEligible("chat", streamReq, false)
	if eligible {
		t.Error("expected streaming request to be ineligible")
	}

	// Chat without temperature (default 1.0) is not eligible when cacheAll is false
	nonZeroTemp := []byte(`{"model":"gpt-4o","messages":[],"temperature":0.7}`)
	eligible, _, _ = IsEligible("chat", nonZeroTemp, false)
	if eligible {
		t.Error("expected temperature > 0 to be ineligible when cacheAll=false")
	}

	// Temperature 0 is eligible
	zeroTemp := []byte(`{"model":"gpt-4o","messages":[],"temperature":0}`)
	eligible, key, err := IsEligible("chat", zeroTemp, false)
	if !eligible || err != nil || key == "" {
		t.Errorf("expected temperature 0 to be eligible, got eligible=%v, err=%v", eligible, err)
	}

	// When cacheAll is true, any non-streaming request is eligible
	eligible, _, _ = IsEligible("chat", nonZeroTemp, true)
	if !eligible {
		t.Error("expected temperature > 0 to be eligible when cacheAll=true")
	}

	// Embeddings are always eligible
	embReq := []byte(`{"model":"text-embedding-3-small","input":"test string"}`)
	eligible, _, _ = IsEligible("embeddings", embReq, false)
	if !eligible {
		t.Error("expected embeddings to be always eligible")
	}
}

func TestShouldBypass(t *testing.T) {
	h := make(http.Header)
	if ShouldBypass(h) {
		t.Error("expected normal headers not to bypass")
	}

	h.Set("Cache-Control", "no-cache")
	if !ShouldBypass(h) {
		t.Error("expected Cache-Control: no-cache to bypass")
	}

	h = make(http.Header)
	h.Set("X-Cache-Control", "no-cache")
	if !ShouldBypass(h) {
		t.Error("expected X-Cache-Control: no-cache to bypass")
	}
}

func TestCacheLRUAndExpiration(t *testing.T) {
	c := New(2, 50*time.Millisecond)

	// Set 2 entries
	c.Set("k1", 200, nil, []byte("body1"), 50*time.Millisecond, "model-1", 10)
	c.Set("k2", 200, nil, []byte("body2"), 50*time.Millisecond, "model-2", 20)

	// Access k1 to make k2 LRU
	entry, hit := c.Get("k1")
	if !hit || string(entry.Body) != "body1" {
		t.Fatalf("expected hit on k1")
	}

	// Insert k3 -> should evict k2 (least recently used)
	c.Set("k3", 200, nil, []byte("body3"), 50*time.Millisecond, "model-3", 30)

	if _, hit := c.Get("k2"); hit {
		t.Error("expected k2 to be evicted by LRU capacity limit")
	}
	if _, hit := c.Get("k1"); !hit {
		t.Error("expected k1 to be retained")
	}
	if _, hit := c.Get("k3"); !hit {
		t.Error("expected k3 to be retained")
	}

	// Wait for TTL expiration
	time.Sleep(60 * time.Millisecond)
	if _, hit := c.Get("k1"); hit {
		t.Error("expected k1 to expire after TTL")
	}

	stats := c.Stats()
	if stats.Hits < 1 {
		t.Errorf("expected at least 1 hit, got %d", stats.Hits)
	}
	if stats.TokensSaved < 10 {
		t.Errorf("expected at least 10 tokens saved, got %d", stats.TokensSaved)
	}
}

func TestResponseRecorder(t *testing.T) {
	rec := httptest.NewRecorder()
	r := NewRecorder(rec)

	r.Header().Set("Content-Type", "application/json")
	r.Header().Set("X-Cyrene-Served-Model", "gpt-4o")
	r.WriteHeader(http.StatusOK)

	payload := map[string]any{
		"choices": []any{
			map[string]any{"message": map[string]any{"content": "hello"}},
		},
		"usage": map[string]any{
			"total_tokens": 42,
		},
	}
	bodyBytes, _ := json.Marshal(payload)
	r.Write(bodyBytes)

	if !r.ShouldCache() {
		t.Error("expected 200 JSON response to be cacheable")
	}

	if r.ExtractTokens() != 42 {
		t.Errorf("expected 42 tokens extracted, got %d", r.ExtractTokens())
	}

	hMap := r.HeaderMap()
	if hMap["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %v", hMap["Content-Type"])
	}
	if hMap["X-Cyrene-Served-Model"] != "gpt-4o" {
		t.Errorf("expected Served Model gpt-4o, got %v", hMap["X-Cyrene-Served-Model"])
	}
}

func TestResponseRecorderOverflow(t *testing.T) {
	rec := httptest.NewRecorder()
	// 100 bytes limit
	r := NewRecorderWithLimit(rec, 100)
	r.WriteHeader(http.StatusOK)

	// Writing 60 bytes: OK
	r.Write(make([]byte, 60))
	if !r.ShouldCache() {
		t.Fatal("expected 60 bytes within 100 limit to be cacheable")
	}

	// Writing 50 more bytes: exceeds 100 limit -> overflow triggered and buffer wiped
	r.Write(make([]byte, 50))
	if r.ShouldCache() {
		t.Fatal("expected overflowed recorder not to be cacheable")
	}
	if len(r.BodyBytes()) != 0 {
		t.Fatalf("expected overflowed buffer to be cleared, got %d bytes", len(r.BodyBytes()))
	}
}

func TestCacheMaxEntryBytes(t *testing.T) {
	c := New(10, time.Hour)
	oversized := make([]byte, MaxEntryBytes+10)
	c.Set("k-huge", 200, nil, oversized, time.Hour, "huge-model", 0)

	if _, hit := c.Get("k-huge"); hit {
		t.Fatal("expected oversized entry to be rejected by Set")
	}
}

func TestCacheGetSnapshotStable(t *testing.T) {
	c := New(2, time.Hour)
	c.Set("key", 200, map[string]string{"version": "old"}, []byte("old"), 0, "old", 10)
	first, _ := c.Get("key")
	before := *first
	c.Get("key")
	c.Set("key", 201, map[string]string{"version": "new"}, []byte("new body"), time.Minute, "new", 20)
	if !reflect.DeepEqual(*first, before) {
		t.Fatalf("previous Get changed after Get/Set: before=%+v after=%+v", before, *first)
	}
	latest, _ := c.Get("key")
	if latest.StatusCode != 201 || string(latest.Body) != "new body" || latest.Headers["version"] != "new" || latest.ServedModel != "new" || latest.TokensSaved != 20 {
		t.Fatalf("replacement not visible: %+v", latest)
	}
	if latest.Hits != 3 || !latest.CreatedAt.Equal(before.CreatedAt) {
		t.Fatalf("replacement lost lifetime metadata: %+v", latest)
	}
	if stats := c.Stats(); stats.BytesUsed != 8 || stats.Hits != 3 || stats.TokensSaved != 40 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestCacheSetOwnsInputs(t *testing.T) {
	c := New(2, time.Hour)
	for _, bodyText := range []string{"first", "replacement"} {
		headers := map[string]string{"version": bodyText}
		body := []byte(bodyText)
		c.Set("key", 200, headers, body, 0, "model", 0)
		headers["version"] = "mutated"
		body[0] = '!'
		entry, _ := c.Get("key")
		if entry.Headers["version"] != bodyText || string(entry.Body) != bodyText {
			t.Fatalf("cache retained caller-owned inputs: %+v", entry)
		}
	}
}

func TestCacheGetOwnsPayload(t *testing.T) {
	c := New(2, time.Hour)
	c.Set("key", 200, map[string]string{"version": "old"}, []byte("old"), 0, "model", 0)
	first, _ := c.Get("key")
	second, _ := c.Get("key")
	first.Headers["version"] = "mutated"
	first.Body[0] = '!'
	first.StatusCode = 500
	third, _ := c.Get("key")
	for _, entry := range []*Entry{second, third} {
		if entry.Headers["version"] != "old" || string(entry.Body) != "old" || entry.StatusCode != 200 {
			t.Fatalf("Get results share mutable storage: %+v", entry)
		}
	}
}

func TestCacheConcurrentGetSet(t *testing.T) {
	c := New(2, time.Hour)
	c.Set("key", 200, map[string]string{"version": "old"}, []byte("old"), 0, "old", 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range 200 {
			version := fmt.Sprint(i)
			c.Set("key", 201, map[string]string{"version": version}, []byte(version), 0, version, i)
		}
	})
	for range 4 {
		wg.Go(func() {
			<-start
			for range 200 {
				entry, hit := c.Get("key")
				if !hit {
					t.Error("unexpected miss")
					return
				}
				before := *entry
				c.Get("key")
				if !reflect.DeepEqual(*entry, before) || string(entry.Body) != entry.ServedModel || entry.Headers["version"] != entry.ServedModel {
					t.Error("inconsistent or mutable snapshot")
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
}

func TestIsEligibleKeyCompatibility(t *testing.T) {
	for _, endpoint := range []string{"chat", "messages", "embeddings"} {
		for _, raw := range []string{
			`{"temperature":0,"stream":false,"messages":[{"content":"hello","role":"user"}],"seed":1e0}`,
			`{"seed":1,"messages":[{"role":"user","content":"hello"}],"temperature":0}`,
			`{"temperature":-0.0,"stream":null}`, `null`, `{}`, `{"stream":"false"}`,
		} {
			eligible, got, err := IsEligible(endpoint, []byte(raw), true)
			want, keyErr := ComputeKey(endpoint, []byte(raw))
			if !eligible || err != nil || keyErr != nil || got != want {
				t.Errorf("%s %s: eligible=%v key=%q want=%q err=%v/%v", endpoint, raw, eligible, got, want, err, keyErr)
			}
		}
		for _, raw := range []string{`{"stream":true,"temperature":0}`, ""} {
			if eligible, key, err := IsEligible(endpoint, []byte(raw), true); eligible || key != "" || err != nil {
				t.Errorf("ineligible %q: %v %q %v", raw, eligible, key, err)
			}
		}
		for _, raw := range []string{`{`, `[]`, `"text"`} {
			if eligible, _, err := IsEligible(endpoint, []byte(raw), true); eligible || err == nil {
				t.Errorf("invalid input %q accepted", raw)
			}
		}
	}
}

func TestIsEligibleAllocationBudget(t *testing.T) {
	body := []byte(`{"model":"test","temperature":0,"messages":[{"role":"user","content":"hello"}]}`)
	keyAllocs := testing.AllocsPerRun(100, func() { ComputeKey("chat", body) })
	eligibleAllocs := testing.AllocsPerRun(100, func() { IsEligible("chat", body, false) })
	// Under -race, sync.Pool randomly drops JSON encoder buffers. Allow that
	// noise, but not the 21 extra allocations from decoding this body twice.
	if eligibleAllocs > keyAllocs+4 {
		t.Fatalf("eligibility allocates %.0f vs key %.0f; must reuse decoded request", eligibleAllocs, keyAllocs)
	}
}

func BenchmarkCacheGetSnapshot(b *testing.B) {
	for _, size := range []int{1024, 65536} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			c := New(2, time.Hour)
			c.Set("key", 200, map[string]string{"Content-Type": "application/json"}, bytes.Repeat([]byte("x"), size), 0, "model", 10)
			b.ReportAllocs()
			for b.Loop() {
				if _, ok := c.Get("key"); !ok {
					b.Fatal("unexpected miss")
				}
			}
		})
	}
}

func BenchmarkIsEligible(b *testing.B) {
	for _, endpoint := range []string{"chat", "embeddings"} {
		b.Run(endpoint, func(b *testing.B) {
			body := []byte(`{"model":"test","temperature":0,"stream":false,"messages":[{"role":"user","content":"hello"}],"input":"hello"}`)
			b.ReportAllocs()
			for b.Loop() {
				if ok, _, err := IsEligible(endpoint, body, false); !ok || err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
