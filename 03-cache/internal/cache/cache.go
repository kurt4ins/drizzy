package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultTTL = 60 * time.Second

type Cache struct {
	client *redis.Client
	ttl    time.Duration
}

func New(addr string) *Cache {
	return &Cache{
		client: redis.NewClient(&redis.Options{Addr: addr}),
		ttl:    defaultTTL,
	}
}

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Cache) Close() error {
	return c.client.Close()
}

func (c *Cache) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (c *Cache) Set(ctx context.Context, key, value string) error {
	return c.client.Set(ctx, key, value, c.ttl).Err()
}

// SetNoTTL stores without expiry — used by write-back so dirty entries persist until flushed.
func (c *Cache) SetNoTTL(ctx context.Context, key, value string) error {
	return c.client.Set(ctx, key, value, 0).Err()
}

func (c *Cache) Del(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

func (c *Cache) Flush(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}
