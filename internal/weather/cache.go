package weather

import (
	"context"
	"sync"
	"time"

	"sprinklergo/internal/model"
)

// Cache decouples weather fetching from the scheduling engine: a background
// loop (or an explicit Refresh) talks to the provider, while the engine and
// the API read the last known result without blocking. The original fetched
// synchronously right before every schedule start.
type Cache struct {
	settings func() model.Settings

	mu        sync.Mutex
	provider  string
	vals      ReturnVals
	scale     int
	fetchedAt time.Time
}

// CacheInfo is a snapshot of the cached weather state.
type CacheInfo struct {
	Provider  string     `json:"provider"`
	Scale     int        `json:"scale"`
	Valid     bool       `json:"valid"`
	FetchedAt int64      `json:"fetchedAt"` // unix seconds, 0 = never
	Vals      ReturnVals `json:"vals"`
}

func NewCache(settings func() model.Settings) *Cache {
	return &Cache{settings: settings, provider: "none", scale: 100}
}

// Scale returns the cached watering scale percentage; neutral 100 until the
// first successful fetch or when no provider is configured.
func (c *Cache) Scale() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scale
}

func (c *Cache) Snapshot() CacheInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	info := CacheInfo{Provider: c.provider, Scale: c.scale, Valid: c.vals.Valid, Vals: c.vals}
	if !c.fetchedAt.IsZero() {
		info.FetchedAt = c.fetchedAt.Unix()
	}
	return info
}

// Refresh fetches from the configured provider and updates the cache.
func (c *Cache) Refresh(ctx context.Context) CacheInfo {
	s := c.settings()
	p := ForSettings(s)
	var vals ReturnVals
	if p.Name() != "none" {
		vals = p.GetVals(ctx, s)
	}
	c.mu.Lock()
	c.provider = p.Name()
	c.vals = vals
	c.scale = Scale(vals)
	c.fetchedAt = time.Now()
	c.mu.Unlock()
	return c.Snapshot()
}

// Run refreshes immediately and then on the given interval until ctx ends.
func (c *Cache) Run(ctx context.Context, every time.Duration) {
	for {
		fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		c.Refresh(fetchCtx)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
}
