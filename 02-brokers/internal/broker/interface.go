package broker

import (
	"context"
	"time"
)

type Broker interface {
	Publish(ctx context.Context, payload []byte) error
	Subscribe(ctx context.Context, handler func([]byte, time.Time)) error
	Close() error
}
