package discovery

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Queue struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Queue { return &Queue{rdb: rdb} }

func (q *Queue) Next(ctx context.Context, viewerUserID string) (string, error) {
	key := fmt.Sprintf("discovery:queue:%s", viewerUserID)
	val, err := q.rdb.LPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lpop discovery queue: %w", err)
	}
	return val, nil
}
