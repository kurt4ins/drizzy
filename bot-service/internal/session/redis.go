package session

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const SessionTTL = 10 * time.Minute

type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func key(telegramID int64) string {
	return fmt.Sprintf("session:%d", telegramID)
}

// SetField sets a single field in the session hash and refreshes the TTL.
func (s *Store) SetField(ctx context.Context, telegramID int64, field, value string) error {
	k := key(telegramID)
	pipe := s.rdb.Pipeline()
	pipe.HSet(ctx, k, field, value)
	pipe.Expire(ctx, k, SessionTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// GetAll returns all fields of the session hash.
// Returns an empty map (not an error) if the session does not exist.
func (s *Store) GetAll(ctx context.Context, telegramID int64) (map[string]string, error) {
	return s.rdb.HGetAll(ctx, key(telegramID)).Result()
}

// Del deletes the session.
func (s *Store) Del(ctx context.Context, telegramID int64) error {
	return s.rdb.Del(ctx, key(telegramID)).Err()
}
