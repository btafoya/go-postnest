package admin

import (
	"context"
	"sync"
	"time"
)

// SettingsCache caches system_settings with TTL refresh.
type SettingsCache struct {
	store  Store
	mu     sync.RWMutex
	data   map[string]string
	loaded time.Time
	ttl    time.Duration
}

// NewSettingsCache creates a settings cache with the given TTL.
func NewSettingsCache(store Store, ttl time.Duration) *SettingsCache {
	return &SettingsCache{
		store: store,
		ttl:   ttl,
	}
}

// Get returns a setting value by key, refreshing from DB on TTL expiry.
func (c *SettingsCache) Get(ctx context.Context, key string) string {
	c.mu.RLock()
	if time.Since(c.loaded) < c.ttl && c.data != nil {
		v := c.data[key]
		c.mu.RUnlock()
		return v
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.loaded) < c.ttl && c.data != nil {
		return c.data[key]
	}
	settings, err := c.store.GetSettings(ctx)
	if err != nil {
		if c.data != nil {
			return c.data[key]
		}
		return ""
	}
	c.data = settings
	c.loaded = time.Now()
	return c.data[key]
}
