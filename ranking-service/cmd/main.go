package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/kurt4ins/drizzy/pkg/config"
	"github.com/kurt4ins/drizzy/pkg/models"
	"github.com/kurt4ins/drizzy/pkg/rabbitmq"
	"github.com/kurt4ins/drizzy/ranking-service/internal/consumer"
	"github.com/kurt4ins/drizzy/ranking-service/internal/repository"
	"github.com/kurt4ins/drizzy/ranking-service/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	postgresDSN := config.Get("POSTGRES_DSN")
	redisAddr := config.Get("REDIS_ADDR")
	rabbitURL := config.Get("RABBITMQ_URL")

	pool, err := pgxpool.New(ctx, postgresDSN)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err = rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("ping redis: %v", err)
	}
	defer rdb.Close()

	repo := repository.New(pool)

	pub, err := rabbitmq.NewPublisher(rabbitURL)
	if err != nil {
		log.Fatalf("rabbitmq publisher: %v", err)
	}
	defer pub.Close()

	interactionConsumer, err := consumer.NewInteractionConsumer(repo, pub, rabbitURL)
	if err != nil {
		log.Fatalf("interaction consumer: %v", err)
	}
	defer interactionConsumer.Close()

	rankingWorker := worker.NewRankingWorker(repo, rdb)

	mux := asynq.NewServeMux()
	mux.HandleFunc(worker.TypeRecalculateRankings, rankingWorker.ProcessTask)

	asynqServer := worker.NewServer(redisAddr)
	if err = asynqServer.Start(mux); err != nil {
		log.Fatalf("asynq server start: %v", err)
	}
	defer asynqServer.Shutdown()

	scheduler := worker.NewScheduler(redisAddr)
	if err = scheduler.Start(); err != nil {
		log.Fatalf("asynq scheduler start: %v", err)
	}
	defer scheduler.Shutdown()

	go func() {
		port := "8081"
		if p := os.Getenv("PORT"); p != "" {
			port = p
		}
		muxHTTP := http.NewServeMux()
		muxHTTP.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		muxHTTP.HandleFunc("/internal/queue/refill", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			var body struct {
				UserID string `json:"user_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" {
				http.Error(w, "bad request: user_id required", http.StatusBadRequest)
				return
			}
			if err := rankingWorker.RefillForUser(r.Context(), body.UserID); err != nil {
				log.Printf("queue refill for %s: %v", body.UserID, err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
		muxHTTP.HandleFunc("/api/v1/users/", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/users/"), "/")
			parts := strings.Split(rest, "/")
			if len(parts) != 2 || parts[1] != "matches" || parts[0] == "" {
				http.NotFound(w, r)
				return
			}
			list, err := repo.ListMatchesForUser(r.Context(), parts[0])
			if err != nil {
				log.Printf("list matches for %s: %v", parts[0], err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if list == nil {
				list = []models.UserMatchEntry{}
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(list); err != nil {
				log.Printf("encode matches: %v", err)
			}
		})
		log.Printf("ranking-service HTTP on :%s", port)
		if err := http.ListenAndServe(":"+port, muxHTTP); err != nil {
			log.Printf("HTTP server: %v", err)
		}
	}()

	go func() {
		if err := interactionConsumer.Run(ctx); err != nil {
			log.Printf("interaction consumer stopped: %v", err)
			stop()
		}
	}()

	log.Println("ranking-service started")
	<-ctx.Done()
	log.Println("ranking-service shutting down")
}
