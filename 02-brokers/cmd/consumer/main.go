package main

import (
	"context"
	"encoding/binary"
	"flag"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"brokers/internal/broker"
	"brokers/internal/metrics"
)

func main() {
	brokerType := flag.String("broker", "rabbitmq", "broker to use: rabbitmq or redis")
	numWorkers := flag.Int("consumers", 4, "number of concurrent consumer goroutines")
	duration := flag.Duration("duration", 30*time.Second, "how long to run")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	col := metrics.New()

	var wg sync.WaitGroup
	for i := 0; i < *numWorkers; i++ {
		wg.Go(func() {
			// Each goroutine gets its own broker connection.
			// Sharing one channel across goroutines is not safe for amqp091-go.
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
				log.Printf("connect error: %v", err)
				return
			}
			defer b.Close()

			err = b.Subscribe(ctx, func(payload []byte, _ time.Time) {
				if len(payload) < 8 {
					return
				}
				nanos := int64(binary.LittleEndian.Uint64(payload[:8]))
				sentAt := time.Unix(0, nanos)
				col.RecordLatency(time.Since(sentAt))
			})
			if err != nil {
				log.Printf("subscribe error: %v", err)
			}
		})
	}

	wg.Wait()

	// Print with rate=0 since the consumer doesn't control the rate.
	col.Summarize().Print(*brokerType, "n/a", 0)
}
