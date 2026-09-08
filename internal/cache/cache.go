package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Entry represents a cached HTTP response.
type Entry struct {
	Key         string
	StatusCode  int
	Headers     map[string]string
	Body        []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
	ServedModel string
	TokensSaved int
	Hits        int64
}

// Stats holds operational telemetry for the cache.
type Stats struct {
	Hits        uint64  `json:"hits"`
	Misses      uint64  `json:"misses"`
	HitRate     float64 `json:"hitRate"`
	Entries     int     `json:"entries"`
	MaxEntries  int     `json:"maxEntries"`
	BytesUsed   int64   `json:"bytesUsed"`
	TokensSaved uint64  `json:"tokensSaved"`
}

// Cache is a thread-safe, bounded in-memory LRU response cache with TTL expiration.
type Cache struct {
	mu          sync.RWMutex
	items       map[string]*list.Element
	evictList   *list.List
	maxEntries  int
	defaultTTL  time.Duration
	hits        atomic.Uint64
	misses      atomic.Uint64
	tokensSaved atomic.Uint64
	bytesUsed   int64
}

// New creates a new Cache with the given maximum entries and default TTL.
func New(maxEntries int, defaultTTL time.Duration) *Cache {
	if maxEntries <= 0 {
		maxEntries = 2000
	}
	if defaultTTL <= 0 {
		defaultTTL = time.Hour
	}
	return &Cache{
		items:      make(map[string]*list.Element),
		evictList:  list.New(),
		maxEntries: maxEntries,
		defaultTTL: defaultTTL,
	}
}

// Get retrieves an entry by key. Returns (nil, false) on miss or expiration.
func (c *Cache) Get(key string) (*Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	entry := elem.Value.(*Entry)
	if time.Now().After(entry.ExpiresAt) {
		// Expired: evict lazily
		c.removeElement(elem)
		c.misses.Add(1)
		return nil, false
	}

	// Move to front of LRU
	c.evictList.MoveToFront(elem)
	entry.Hits++
	c.hits.Add(1)
	if entry.TokensSaved > 0 {
		c.tokensSaved.Add(uint64(entry.TokensSaved))
	}
	return entry, true
}

// Set inserts or updates an entry in the cache.
func (c *Cache) Set(key string, statusCode int, headers map[string]string, body []byte, ttl time.Duration, servedModel string, tokensSaved int) {
	if ttl <= 0 {
		ttl = c.defaultTTL
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	expiresAt := now.Add(ttl)

	// If entry already exists, update and move to front
	if elem, ok := c.items[key]; ok {
		c.evictList.MoveToFront(elem)
		oldEntry := elem.Value.(*Entry)
		c.bytesUsed -= int64(len(oldEntry.Body))
		c.bytesUsed += int64(len(body))

		oldEntry.StatusCode = statusCode
		oldEntry.Headers = headers
		oldEntry.Body = body
		oldEntry.ExpiresAt = expiresAt
		oldEntry.ServedModel = servedModel
		oldEntry.TokensSaved = tokensSaved
		return
	}

	// Evict oldest if full
	for c.evictList.Len() >= c.maxEntries {
		c.removeOldest()
	}

	entry := &Entry{
		Key:         key,
		StatusCode:  statusCode,
		Headers:     headers,
		Body:        body,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		ServedModel: servedModel,
		TokensSaved: tokensSaved,
	}

	elem := c.evictList.PushFront(entry)
	c.items[key] = elem
	c.bytesUsed += int64(len(body))
}

// Delete removes an entry by key.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
		return true
	}
	return false
}

// Clear flushes all entries from the cache while retaining telemetry counters.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
	c.bytesUsed = 0
}

// Stats returns a snapshot of cache metrics.
func (c *Cache) Stats() Stats {
	c.mu.RLock()
	entries := len(c.items)
	bytesUsed := c.bytesUsed
	maxEntries := c.maxEntries
	c.mu.RUnlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return Stats{
		Hits:        hits,
		Misses:      misses,
		HitRate:     hitRate,
		Entries:     entries,
		MaxEntries:  maxEntries,
		BytesUsed:   bytesUsed,
		TokensSaved: c.tokensSaved.Load(),
	}
}

func (c *Cache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	entry := elem.Value.(*Entry)
	delete(c.items, entry.Key)
	c.bytesUsed -= int64(len(entry.Body))
}

func (c *Cache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// ComputeKey derives a deterministic SHA-256 cache key from the normalized request body.
// Strips `stream` so identical requests share cache entries regardless of transport.
func ComputeKey(endpoint string, rawBody []byte) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(rawBody, &m); err != nil {
		return "", err
	}
	delete(m, "stream")

	// json.Marshal on map[string]any sorts keys deterministically in Go
	canonicalJSON, err := json.Marshal(m)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	h.Write([]byte(endpoint))
	h.Write([]byte(":"))
	h.Write(canonicalJSON)
	return endpoint + ":" + hex.EncodeToString(h.Sum(nil)), nil
}

// IsEligible inspects a request payload to determine if it can be cached.
// Non-streaming requests only. When cacheAll is false, chat requests require temperature == 0.
// Embeddings are always eligible as they are deterministic.
func IsEligible(endpoint string, rawBody []byte, cacheAll bool) (bool, string, error) {
	if len(rawBody) == 0 {
		return false, "", nil
	}
	var m map[string]any
	if err := json.Unmarshal(rawBody, &m); err != nil {
		return false, "", err
	}

	// Never cache streaming SSE responses
	if stream, ok := m["stream"].(bool); ok && stream {
		return false, "", nil
	}

	// Embeddings are always deterministic
	if endpoint == "embeddings" {
		key, err := ComputeKey(endpoint, rawBody)
		return err == nil, key, err
	}

	// Chat / Messages
	if !cacheAll {
		temp, hasTemp := m["temperature"]
		if !hasTemp {
			return false, "", nil
		}
		tempFloat, ok := temp.(float64)
		if !ok || tempFloat != 0.0 {
			return false, "", nil
		}
	}

	key, err := ComputeKey(endpoint, rawBody)
	return err == nil, key, err
}

// ShouldBypass checks whether the client explicitly requested to bypass cache via HTTP headers.
func ShouldBypass(h http.Header) bool {
	if strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-cache") {
		return true
	}
	if strings.Contains(strings.ToLower(h.Get("X-Cache-Control")), "no-cache") {
		return true
	}
	if strings.Contains(strings.ToLower(h.Get("Pragma")), "no-cache") {
		return true
	}
	return false
}
