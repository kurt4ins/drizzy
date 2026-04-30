package metrics

import (
	"fmt"
	"sync/atomic"
	"time"
)

type M struct {
	reads, writes, hits, dbr, dbw atomic.Int64
	latNs, latN                   atomic.Int64
}

func (m *M) IncReads()    { m.reads.Add(1) }
func (m *M) IncWrites()   { m.writes.Add(1) }
func (m *M) IncHits()     { m.hits.Add(1) }
func (m *M) IncDBReads()  { m.dbr.Add(1) }
func (m *M) IncDBWrites() { m.dbw.Add(1) }

func (m *M) RecordLat(d time.Duration) {
	m.latNs.Add(int64(d))
	m.latN.Add(1)
}

type Result struct {
	Reads      int64
	Writes     int64
	CacheHits  int64
	DBReads    int64
	DBWrites   int64
	HitRate    float64 // percent of reads served from cache
	AvgLatMs   float64
	Throughput float64 // total RPS
}

func (m *M) Report(elapsed time.Duration) Result {
	reads := m.reads.Load()
	writes := m.writes.Load()
	hits := m.hits.Load()
	dbr := m.dbr.Load()
	dbw := m.dbw.Load()
	latNs := m.latNs.Load()
	latN := m.latN.Load()

	var hitRate float64
	if reads > 0 {
		hitRate = float64(hits) / float64(reads) * 100
	}
	var avgLat float64
	if latN > 0 {
		avgLat = float64(latNs) / float64(latN) / 1e6
	}
	var throughput float64
	if elapsed > 0 {
		throughput = float64(reads+writes) / elapsed.Seconds()
	}

	return Result{
		Reads: reads, Writes: writes,
		CacheHits: hits, DBReads: dbr, DBWrites: dbw,
		HitRate: hitRate, AvgLatMs: avgLat, Throughput: throughput,
	}
}

func (r Result) String() string {
	return fmt.Sprintf("throughput=%.0f RPS  lat=%.3fms  hitRate=%.1f%%  dbReads=%d  dbWrites=%d",
		r.Throughput, r.AvgLatMs, r.HitRate, r.DBReads, r.DBWrites)
}
