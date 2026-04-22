package userstore

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "bot:user:"

type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store { return &Store{rdb: rdb} }

func (s *Store) Save(ctx context.Context, telegramID int64, userID string) error {
	key := fmt.Sprintf("%s%d", keyPrefix, telegramID)
	return s.rdb.Set(ctx, key, userID, 0).Err()
}

func (s *Store) GetUserID(ctx context.Context, telegramID int64) (string, error) {
	key := fmt.Sprintf("%s%d", keyPrefix, telegramID)
	val, err := s.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get user id: %w", err)
	}
	return val, nil
}
