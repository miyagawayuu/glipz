package httpserver

import (
	"sync"
	"time"
)

type feedCache struct {
	mu      sync.Mutex
	expires time.Time
	items   []feedItem
}

func (c *feedCache) get(now time.Time) ([]feedItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) == 0 || now.After(c.expires) {
		return nil, false
	}
	out := append([]feedItem(nil), c.items...)
	return out, true
}

func (c *feedCache) set(now time.Time, ttl time.Duration, items []feedItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expires = now.Add(ttl)
	c.items = append([]feedItem(nil), items...)
}

type responseCacheEntry struct {
	expires time.Time
	value   any
}

type responseCache struct {
	mu      sync.Mutex
	entries map[string]responseCacheEntry
	locks   map[string]*sync.Mutex
}

func (c *responseCache) get(now time.Time, key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		return nil, false
	}
	ent, ok := c.entries[key]
	if !ok || now.After(ent.expires) {
		return nil, false
	}
	return ent.value, true
}

func (c *responseCache) set(now time.Time, key string, ttl time.Duration, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]responseCacheEntry{}
	}
	c.entries[key] = responseCacheEntry{expires: now.Add(ttl), value: value}
}

func (c *responseCache) keyLock(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks == nil {
		c.locks = map[string]*sync.Mutex{}
	}
	if l := c.locks[key]; l != nil {
		return l
	}
	l := &sync.Mutex{}
	c.locks[key] = l
	return l
}

func (c *responseCache) getOrLoad(now time.Time, key string, ttl time.Duration, load func() (any, error)) (any, bool, error) {
	if v, ok := c.get(now, key); ok {
		return v, true, nil
	}
	l := c.keyLock(key)
	l.Lock()
	defer l.Unlock()
	if v, ok := c.get(time.Now(), key); ok {
		return v, true, nil
	}
	v, err := load()
	if err != nil {
		return nil, false, err
	}
	c.set(time.Now(), key, ttl, v)
	return v, false, nil
}
