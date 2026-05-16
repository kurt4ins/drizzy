package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurt4ins/drizzy/profile-service/internal/storage"
	"github.com/redis/go-redis/v9"
)

type HealthChecker struct {
	pool  *pgxpool.Pool
	rdb   *redis.Client
	store *storage.MinIO
}

func NewHealthChecker(pool *pgxpool.Pool, rdb *redis.Client, store *storage.MinIO) *HealthChecker {
	return &HealthChecker{pool: pool, rdb: rdb, store: store}
}

func (h *HealthChecker) Handle(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{"status": "ok"}
	overall := http.StatusOK

	ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		checks["postgres"] = err.Error()
		overall = http.StatusServiceUnavailable
	} else {
		checks["postgres"] = "ok"
	}

	if err := h.rdb.Ping(ctx).Err(); err != nil {
		checks["redis"] = err.Error()
		overall = http.StatusServiceUnavailable
	} else {
		checks["redis"] = "ok"
	}

	if err := h.store.Ping(ctx); err != nil {
		checks["minio"] = err.Error()
		overall = http.StatusServiceUnavailable
	} else {
		checks["minio"] = "ok"
	}

	if overall != http.StatusOK {
		checks["status"] = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(overall)
	_ = json.NewEncoder(w).Encode(checks)
}

// HealthHandler is a backwards-compatible no-dependency handler kept so existing
// callers (and the route signature in main) continue to compile during the
// transition.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
