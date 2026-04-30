package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"cache/internal/cache"
	"cache/internal/db"
	"cache/internal/metrics"
	"cache/internal/store"
)

var (
	dbDSN       = flag.String("db", "postgres://cache:cache@localhost:5432/cache", "PostgreSQL DSN")
	redisAddr   = flag.String("redis", "localhost:6379", "Redis address")
	duration    = flag.Duration("duration", 10*time.Second, "benchmark duration per run")
	concurrency = flag.Int("concurrency", 50, "concurrent workers")
	numKeys     = flag.Int("keys", 1000, "number of keys to seed")
	flushInt    = flag.Duration("flush-interval", 3*time.Second, "write-back flush interval")
)

type workload struct {
	name      string
	readRatio float64
}

var workloads = []workload{
	{"read-heavy  (80/20)", 0.80},
	{"balanced    (50/50)", 0.50},
	{"write-heavy (20/80)", 0.20},
}

type runResult struct {
	strategy string
	workload string
	metrics.Result
	flushEvents  int64
	totalFlushed int64
}

func main() {
	flag.Parse()
	ctx := context.Background()

	log.Printf("connecting to postgres: %s", *dbDSN)
	d, err := db.New(ctx, *dbDSN)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer d.Close()

	if err := d.Init(ctx); err != nil {
		log.Fatalf("db init: %v", err)
	}

	log.Printf("connecting to redis: %s", *redisAddr)
	c := cache.New(*redisAddr)
	if err := c.Ping(ctx); err != nil {
		log.Fatalf("redis connect: %v", err)
	}
	defer c.Close()

	fmt.Printf("\nSettings: keys=%d  duration=%s  concurrency=%d  flush-interval=%s\n\n",
		*numKeys, *duration, *concurrency, *flushInt)

	type strategyEntry struct {
		name string
		make func(*db.DB, *cache.Cache, *metrics.M) (store.Store, *store.WriteBack)
	}

	strategies := []strategyEntry{
		{"lazy", func(d *db.DB, c *cache.Cache, m *metrics.M) (store.Store, *store.WriteBack) {
			return store.NewLazy(d, c, m), nil
		}},
		{"write-through", func(d *db.DB, c *cache.Cache, m *metrics.M) (store.Store, *store.WriteBack) {
			return store.NewWriteThrough(d, c, m), nil
		}},
		{"write-back", func(d *db.DB, c *cache.Cache, m *metrics.M) (store.Store, *store.WriteBack) {
			wb := store.NewWriteBack(d, c, m, *flushInt)
			return wb, wb
		}},
	}

	var results []runResult

	for _, strat := range strategies {
		for _, wl := range workloads {
			log.Printf("▶  %-14s  %s", strat.name, wl.name)

			if err := d.Reset(ctx); err != nil {
				log.Fatalf("db reset: %v", err)
			}
			if err := d.Seed(ctx, *numKeys); err != nil {
				log.Fatalf("db seed: %v", err)
			}
			if err := c.Flush(ctx); err != nil {
				log.Fatalf("cache flush: %v", err)
			}

			m := &metrics.M{}
			s, wb := strat.make(d, c, m)

			start := time.Now()
			runBenchmark(ctx, s, wl.readRatio, *numKeys, *duration, *concurrency)
			elapsed := time.Since(start)

			if err := s.Close(ctx); err != nil {
				log.Printf("close error: %v", err)
			}

			r := runResult{
				strategy: strat.name,
				workload: wl.name,
				Result:   m.Report(elapsed),
			}
			if wb != nil {
				r.flushEvents = wb.FlushEvents()
				r.totalFlushed = wb.TotalFlushed()
			}

			log.Printf("   %s", r.Result)
			results = append(results, r)
		}
	}

	printTable(results)
}

func runBenchmark(ctx context.Context, s store.Store, readRatio float64, numKeys int, dur time.Duration, concurrency int) {
	ctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(id)))
			for {
				if ctx.Err() != nil {
					return
				}
				key := fmt.Sprintf("key:%d", rng.IntN(numKeys)+1)
				if rng.Float64() < readRatio {
					_, _ = s.Get(ctx, key)
				} else {
					_ = s.Set(ctx, key, fmt.Sprintf("v%d", rng.Int64()))
				}
			}
		}(i)
	}
	wg.Wait()
}

func printTable(results []runResult) {
	sep := strings.Repeat("─", 100)

	fmt.Println("\n" + sep)
	fmt.Printf("%-14s  %-22s  %10s  %9s  %9s  %10s  %10s\n",
		"Strategy", "Workload", "RPS", "Lat (ms)", "HitRate%", "DB Reads", "DB Writes")
	fmt.Println(sep)

	for _, r := range results {
		extra := ""
		if r.flushEvents > 0 {
			avg := float64(r.totalFlushed) / float64(r.flushEvents)
			extra = fmt.Sprintf("  (flushes=%d  avg_dirty=%.1f)", r.flushEvents, avg)
		}
		fmt.Printf("%-14s  %-22s  %10.0f  %9.3f  %9.1f  %10d  %10d%s\n",
			r.strategy, r.workload,
			r.Throughput, r.AvgLatMs, r.HitRate,
			r.DBReads, r.DBWrites,
			extra,
		)
	}
	fmt.Println(sep)
}
