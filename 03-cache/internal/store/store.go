package store

import "context"

type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	// Close flushes any pending state (write-back dirty buffer) and releases resources.
	Close(ctx context.Context) error
}
