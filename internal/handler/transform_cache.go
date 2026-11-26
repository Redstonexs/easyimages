package handler

import (
	"container/list"
	"sync"
	"time"
)

const (
	maxCachedTransformSize = 1 << 20
	maxTransformCacheBytes = 32 << 20
	maxTransformEntries    = 128
	transformCacheTTL      = 5 * time.Minute
)

type cachedTransform struct {
	key       string
	data      []byte
	expiresAt time.Time
}

type transformCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	lru     list.List
	bytes   int
}

func newTransformCache() *transformCache {
	return &transformCache{entries: make(map[string]*list.Element)}
}

func (c *transformCache) get(key string, now time.Time) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	element := c.entries[key]
	if element == nil {
		return nil
	}
	entry := element.Value.(cachedTransform)
	if !now.Before(entry.expiresAt) {
		c.remove(element)
		return nil
	}
	c.lru.MoveToFront(element)
	return entry.data
}

func (c *transformCache) put(key string, data []byte, now time.Time) {
	if len(data) == 0 || len(data) > maxCachedTransformSize {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.entries[key]; old != nil {
		c.remove(old)
	}
	c.entries[key] = c.lru.PushFront(cachedTransform{key: key, data: data, expiresAt: now.Add(transformCacheTTL)})
	c.bytes += len(data)
	for len(c.entries) > maxTransformEntries || c.bytes > maxTransformCacheBytes {
		c.remove(c.lru.Back())
	}
}

func (c *transformCache) remove(element *list.Element) {
	entry := element.Value.(cachedTransform)
	delete(c.entries, entry.key)
	c.bytes -= len(entry.data)
	c.lru.Remove(element)
}
