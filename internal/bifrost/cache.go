package bifrost

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sync"
	"time"
)

type cacheEntry struct {
	page      renderedPage
	expiresAt time.Time
}

type renderCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

func newRenderCache(ttl time.Duration) *renderCache {
	return &renderCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

func (c *renderCache) get(key string) (renderedPage, bool) {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return renderedPage{}, false
	}

	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return renderedPage{}, false
	}

	return entry.page, true
}

func (c *renderCache) set(key string, page renderedPage) {
	c.mu.Lock()
	c.entries[key] = cacheEntry{
		page:      page,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *renderCache) clear() {
	c.mu.Lock()
	c.entries = make(map[string]cacheEntry)
	c.mu.Unlock()
}

func hashProps(props map[string]interface{}) (string, error) {
	if props == nil {
		return "nil", nil
	}
	data, err := json.Marshal(props)
	if err != nil {
		return "", fmt.Errorf("failed to marshal props for cache key: %w", err)
	}
	h := fnv.New64a()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum64()), nil
}
