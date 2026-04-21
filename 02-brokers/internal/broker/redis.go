package broker

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisStream    = "bench"
	redisGroup     = "bench-group"
	redisConsumer  = "bench-consumer"
	redisBodyField = "body"
)

type Redis struct {
	client *redis.Client
}

func NewRedis(addr string) (*Redis, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, err
	}
	err := client.XGroupCreateMkStream(context.Background(), redisStream, redisGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return nil, err
	}
	return &Redis{client: client}, nil
}

func (r *Redis) Publish(ctx context.Context, payload []byte) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: redisStream,
		Values: map[string]any{redisBodyField: payload},
	}).Err()
}

func (r *Redis) Subscribe(ctx context.Context, handler func([]byte, time.Time)) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    redisGroup,
			Consumer: redisConsumer,
			Streams:  []string{redisStream, ">"},
			Count:    100,
			Block:    time.Second,
		}).Result()
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return nil
			}
			continue
		}
		for _, stream := range streams {
			for _, msg := range stream.Messages {
				body, _ := msg.Values[redisBodyField].(string)
				handler([]byte(body), time.Time{})
				r.client.XAck(ctx, redisStream, redisGroup, msg.ID)
			}
		}
	}
}

func (r *Redis) Close() error {
	return r.client.Close()
}
