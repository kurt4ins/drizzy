package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/kurt4ins/drizzy/ranking-service/internal/repository"
)

const (
	TypeRecalculateRankings = "ranking:recalculate"
	discoveryQueueTTL       = 30 * time.Minute
	discoveryQueueSize      = 10
)

type RankingWorker struct {
	repo *repository.Repository
	rdb  *redis.Client
}

func NewRankingWorker(repo *repository.Repository, rdb *redis.Client) *RankingWorker {
	return &RankingWorker{repo: repo, rdb: rdb}
}

func (w *RankingWorker) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	start := time.Now()
	log.Println("ranking worker: starting score recalculation")

	if err := w.repo.RecalculateAllScores(ctx); err != nil {
		return fmt.Errorf("recalculate scores: %w", err)
	}
	log.Printf("ranking worker: scores recalculated in %s", time.Since(start))

	if err := w.refillDiscoveryQueues(ctx); err != nil {
		return fmt.Errorf("refill queues: %w", err)
	}
	log.Printf("ranking worker: finished in %s", time.Since(start))
	return nil
}

func (w *RankingWorker) refillDiscoveryQueues(ctx context.Context) error {
	userIDs, err := w.repo.ActiveUserIDs(ctx)
	if err != nil {
		return err
	}

	for _, uid := range userIDs {
		if err := w.RefillForUser(ctx, uid); err != nil {
			log.Printf("ranking worker: refill queue for %s: %v", uid, err)
		}
	}
	return nil
}

func (w *RankingWorker) RefillForUser(ctx context.Context, userID string) error {
	candidates, err := w.repo.TopCandidates(ctx, userID, discoveryQueueSize)
	if err != nil {
		return fmt.Errorf("top candidates for %s: %w", userID, err)
	}
	if len(candidates) == 0 {
		return nil
	}

	key := fmt.Sprintf("discovery:queue:%s", userID)
	pipe := w.rdb.Pipeline()
	pipe.Del(ctx, key)
	vals := make([]any, len(candidates))
	for i, c := range candidates {
		vals[i] = c
	}
	pipe.RPush(ctx, key, vals...)
	pipe.Expire(ctx, key, discoveryQueueTTL)
	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline for %s: %w", userID, err)
	}
	return nil
}

func NewScheduler(redisAddr string) *asynq.Scheduler {
	s := asynq.NewScheduler(
		asynq.RedisClientOpt{Addr: redisAddr},
		&asynq.SchedulerOpts{
			Location: time.UTC,
		},
	)
	_, _ = s.Register("*/1 * * * *", asynq.NewTask(TypeRecalculateRankings, nil))
	return s
}

func NewServer(redisAddr string) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 2,
			Queues:      map[string]int{"default": 1},
		},
	)
}
