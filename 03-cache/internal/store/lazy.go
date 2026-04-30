package store

import (
	"context"
	"time"

	"cache/internal/cache"
	"cache/internal/db"
	"cache/internal/metrics"
)

// Lazy implements Cache-Aside / Lazy Loading / Write-Around:
//   - reads: check cache → on miss, load from DB and populate cache
//   - writes: go directly to DB, invalidate cache entry
type Lazy struct {
	db    *db.DB
	cache *cache.Cache
	m     *metrics.M
}

func NewLazy(d *db.DB, c *cache.Cache, m *metrics.M) *Lazy {
	return &Lazy{db: d, cache: c, m: m}
}

func (l *Lazy) Get(ctx context.Context, key string) (string, error) {
	start := time.Now()
	l.m.IncReads()

	val, hit, err := l.cache.Get(ctx, key)
	if err == nil && hit {
		l.m.IncHits()
		l.m.RecordLat(time.Since(start))
		return val, nil
	}

	l.m.IncDBReads()
	val, found, err := l.db.Get(ctx, key)
	l.m.RecordLat(time.Since(start))
	if err != nil {
		return "", err
	}
	if found {
		_ = l.cache.Set(ctx, key, val)
	}
	return val, nil
}

func (l *Lazy) Set(ctx context.Context, key, value string) error {
	start := time.Now()
	l.m.IncWrites()

	_ = l.cache.Del(ctx, key)

	l.m.IncDBWrites()
	err := l.db.Set(ctx, key, value)
	l.m.RecordLat(time.Since(start))
	return err
}

func (l *Lazy) Close(_ context.Context) error { return nil }
