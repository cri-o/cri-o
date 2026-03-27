package supplychain

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// cacheKey combines image digest and namespace for per-namespace caching.
type cacheKey struct {
	digest    string
	namespace string
}

// cacheEntry holds a cached verification result with expiry.
type cacheEntry struct {
	result    *Result
	expiresAt time.Time
}

// maxCacheSize is the maximum number of entries allowed in the cache.
// This prevents unbounded memory growth on nodes with many unique images.
const maxCacheSize = 10000

// Cache stores supply chain verification results with TTL-based expiry.
type Cache struct {
	mu      sync.Mutex
	entries map[cacheKey]cacheEntry
	ttl     time.Duration
}

// NewCache creates a new verification result cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[cacheKey]cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a cached result for the given digest and namespace.
// Returns nil if no valid cache entry exists. Expired entries are evicted.
func (c *Cache) Get(digest, namespace string) *Result {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := cacheKey{digest: digest, namespace: namespace}

	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)

		return nil
	}

	return entry.result
}

// Set stores a verification result in the cache.
func (c *Cache) Set(digest, namespace string, result *Result) {
	if c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries if the cache is at capacity.
	if len(c.entries) >= maxCacheSize {
		c.evictExpiredLocked()
	}

	// If still at capacity after eviction, skip caching this entry
	// rather than growing unboundedly.
	if len(c.entries) >= maxCacheSize {
		logrus.WithFields(logrus.Fields{
			"capacity":  maxCacheSize,
			"namespace": namespace,
			"digest":    digest,
		}).Warn("Supply chain verification cache at capacity, dropping entry")

		return
	}

	key := cacheKey{digest: digest, namespace: namespace}
	c.entries[key] = cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// evictExpiredLocked removes all expired entries. Caller must hold c.mu.
func (c *Cache) evictExpiredLocked() {
	now := time.Now()

	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

// Clear removes all cached entries. Used in tests.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[cacheKey]cacheEntry)
}
