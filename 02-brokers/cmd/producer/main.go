package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"brokers/internal/broker"
	"brokers/internal/metrics"

	"golang.org/x/time/rate"
)

func main() {
	brokerType := flag.String("broker", "rabbitmq", "broker to use: rabbitmq or redis")
	msgSize := flag.Int("size", 1024, "message payload size in bytes")
	rateLimit := flag.Int("rate", 1000, "total messages per second across all producers")
	duration := flag.Duration("duration", 30*time.Second, "how long to run")
	numWorkers := flag.Int("producers", 4, "number of concurrent producer goroutines")
	flag.Parse()

	var b broker.Broker
	var err error
	switch *brokerType {
	case "rabbitmq":
		b, err = broker.NewRabbitMQ("amqp://guest:guest@localhost:5672/")
	case "redis":
		b, err = broker.NewRedis("localhost:6379")
	default:
		log.Fatalf("unknown broker: %s", *brokerType)
	}
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer b.Close()

	col := metrics.New()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	// Catch Ctrl-C too
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	// Rate is shared across all goroutines via a single limiter.
	// Burst = numWorkers so they can all fire at once without stalling.
	limiter := rate.NewLimiter(rate.Limit(*rateLimit), *numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < *numWorkers; i++ {
		wg.Go(func() {
			payload := make([]byte, *msgSize)
			for {
				if ctx.Err() != nil {
					return
				}
				if err := limiter.Wait(ctx); err != nil {
					return // context done
				}

				// Stamp send time in first 8 bytes so consumer can measure latency.
				// binary.LittleEndian lets the consumer decode it without reflection.
				binary.LittleEndian.PutUint64(payload[:8], uint64(time.Now().UnixNano()))
				// Fill the rest with random bytes to avoid compression artifacts.
				rand.Read(payload[8:])

				if err := b.Publish(ctx, payload); err != nil {
					col.RecordError()
				} else {
					col.RecordSent()
				}
			}
		})
	}

	wg.Wait()

	sizeLabel := fmt.Sprintf("%dB", *msgSize)
	col.Summarize().Print(*brokerType, sizeLabel, *rateLimit)
}
