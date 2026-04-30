package store

import (
	"context"
	"time"

	"cache/internal/cache"
	"cache/internal/db"
	"cache/internal/metrics"
)

// WriteThrough writes to cache and DB synchronously on every write.
// Reads are served from cache; misses fall through to DB and populate cache.
type WriteThrough struct {
	db    *db.DB
	cache *cache.Cache
	m     *metrics.M
}

func NewWriteThrough(d *db.DB, c *cache.Cache, m *metrics.M) *WriteThrough {
	return &WriteThrough{db: d, cache: c, m: m}
}

func (wt *WriteThrough) Get(ctx context.Context, key string) (string, error) {
	start := time.Now()
	wt.m.IncReads()

	val, hit, err := wt.cache.Get(ctx, key)
	if err == nil && hit {
		wt.m.IncHits()
		wt.m.RecordLat(time.Since(start))
		return val, nil
	}

	wt.m.IncDBReads()
	val, found, err := wt.db.Get(ctx, key)
	wt.m.RecordLat(time.Since(start))
	if err != nil {
		return "", err
	}
	if found {
		_ = wt.cache.Set(ctx, key, val)
	}
	return val, nil
}

func (wt *WriteThrough) Set(ctx context.Context, key, value string) error {
	start := time.Now()
	wt.m.IncWrites()

	_ = wt.cache.Set(ctx, key, value)

	wt.m.IncDBWrites()
	err := wt.db.Set(ctx, key, value)
	wt.m.RecordLat(time.Since(start))
	return err
}

func (wt *WriteThrough) Close(_ context.Context) error { return nil }
