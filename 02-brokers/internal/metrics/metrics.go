package metrics

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

type Collector struct {
	Sent     atomic.Int64
	Received atomic.Int64
	Errors   atomic.Int64

	mu   sync.Mutex
	hist *hdrhistogram.Histogram

	startedAt time.Time
}

func New() *Collector {
	return &Collector{
		hist:      hdrhistogram.New(1, 30_000_000, 3),
		startedAt: time.Now(),
	}
}

func (c *Collector) RecordLatency(d time.Duration) {
	c.Received.Add(1)
	micros := d.Microseconds()
	c.mu.Lock()
	_ = c.hist.RecordValue(micros)
	c.mu.Unlock()
}

func (c *Collector) RecordSent() { c.Sent.Add(1) }

func (c *Collector) RecordError() { c.Errors.Add(1) }

type Summary struct {
	Duration           time.Duration
	Sent               int64
	Received           int64
	Lost               int64
	Errors             int64
	Throughput         float64
	P50, P95, P99, Max time.Duration
}

func (c *Collector) Summarize() Summary {
	elapsed := time.Since(c.startedAt)
	sent := c.Sent.Load()
	recv := c.Received.Load()
	errs := c.Errors.Load()

	c.mu.Lock()
	p50 := time.Duration(c.hist.ValueAtQuantile(50)) * time.Microsecond
	p95 := time.Duration(c.hist.ValueAtQuantile(95)) * time.Microsecond
	p99 := time.Duration(c.hist.ValueAtQuantile(99)) * time.Microsecond
	max := time.Duration(c.hist.Max()) * time.Microsecond
	c.mu.Unlock()

	return Summary{
		Duration:   elapsed,
		Sent:       sent,
		Received:   recv,
		Lost:       sent - recv,
		Errors:     errs,
		Throughput: float64(recv) / elapsed.Seconds(),
		P50:        p50, P95: p95, P99: p99, Max: max,
	}
}

func (s Summary) Print(broker, size string, rate int) {
	fmt.Printf(
		"broker=%-10s size=%-8s rate=%-6d dur=%v sent=%d recv=%d lost=%d err=%d "+
			"tput=%.0f msg/s  p50=%v p95=%v p99=%v max=%v\n",
		broker, size, rate, s.Duration.Round(time.Second),
		s.Sent, s.Received, s.Lost, s.Errors,
		s.Throughput,
		s.P50, s.P95, s.P99, s.Max,
	)
}
