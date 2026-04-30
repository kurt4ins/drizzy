package store

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"cache/internal/cache"
	"cache/internal/db"
	"cache/internal/metrics"
)

// WriteBack writes first to cache and a dirty buffer; a background goroutine
// periodically flushes dirty entries to the DB in batches.
type WriteBack struct {
	db       *db.DB
	cache    *cache.Cache
	m        *metrics.M
	flushInt time.Duration

	mu    sync.Mutex
	dirty map[string]string

	flushEvents  atomic.Int64
	totalFlushed atomic.Int64

	done chan struct{}
	wg   sync.WaitGroup
}

func NewWriteBack(d *db.DB, c *cache.Cache, m *metrics.M, flushInterval time.Duration) *WriteBack {
	wb := &WriteBack{
		db: d, cache: c, m: m,
		flushInt: flushInterval,
		dirty:    make(map[string]string),
		done:     make(chan struct{}),
	}
	wb.wg.Add(1)
	go func() {
		defer wb.wg.Done()
		wb.flushLoop()
	}()
	return wb
}

func (wb *WriteBack) FlushEvents() int64  { return wb.flushEvents.Load() }
func (wb *WriteBack) TotalFlushed() int64 { return wb.totalFlushed.Load() }

func (wb *WriteBack) flushLoop() {
	ticker := time.NewTicker(wb.flushInt)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			wb.flush(context.Background())
		case <-wb.done:
			wb.flush(context.Background())
			return
		}
	}
}

func (wb *WriteBack) flush(ctx context.Context) {
	wb.mu.Lock()
	if len(wb.dirty) == 0 {
		wb.mu.Unlock()
		return
	}
	snapshot := wb.dirty
	wb.dirty = make(map[string]string)
	wb.mu.Unlock()

	wb.flushEvents.Add(1)
	wb.totalFlushed.Add(int64(len(snapshot)))

	for k, v := range snapshot {
		wb.m.IncDBWrites()
		_ = wb.db.Set(ctx, k, v)
	}
}

func (wb *WriteBack) Get(ctx context.Context, key string) (string, error) {
	start := time.Now()
	wb.m.IncReads()

	// dirty buffer holds the most recent write — check it first
	wb.mu.Lock()
	if val, ok := wb.dirty[key]; ok {
		wb.mu.Unlock()
		wb.m.IncHits()
		wb.m.RecordLat(time.Since(start))
		return val, nil
	}
	wb.mu.Unlock()

	val, hit, err := wb.cache.Get(ctx, key)
	if err == nil && hit {
		wb.m.IncHits()
		wb.m.RecordLat(time.Since(start))
		return val, nil
	}

	wb.m.IncDBReads()
	val, found, err := wb.db.Get(ctx, key)
	wb.m.RecordLat(time.Since(start))
	if err != nil {
		return "", err
	}
	if found {
		_ = wb.cache.SetNoTTL(ctx, key, val)
	}
	return val, nil
}

func (wb *WriteBack) Set(ctx context.Context, key, value string) error {
	start := time.Now()
	wb.m.IncWrites()

	_ = wb.cache.SetNoTTL(ctx, key, value)

	wb.mu.Lock()
	wb.dirty[key] = value
	wb.mu.Unlock()

	wb.m.RecordLat(time.Since(start))
	return nil
}

func (wb *WriteBack) Close(_ context.Context) error {
	close(wb.done)
	wb.wg.Wait()
	return nil
}
