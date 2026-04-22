package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurt4ins/drizzy/pkg/config"
	"github.com/kurt4ins/drizzy/profile-service/internal/handler"
	"github.com/kurt4ins/drizzy/profile-service/internal/repository"
	"github.com/kurt4ins/drizzy/profile-service/internal/storage"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx := context.Background()

	postgresDSN := config.Get("POSTGRES_DSN")
	redisAddr := config.Get("REDIS_ADDR")
	minioEndpoint := config.Get("MINIO_ENDPOINT")
	minioAccessKey := config.Get("MINIO_ACCESS_KEY")
	minioSecretKey := config.Get("MINIO_SECRET_KEY")

	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

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

	minioStore, err := storage.New(minioEndpoint, minioAccessKey, minioSecretKey, false)
	if err != nil {
		log.Fatalf("minio client: %v", err)
	}
	if err = minioStore.EnsureBucket(ctx); err != nil {
		log.Fatalf("minio ensure bucket: %v", err)
	}

	userRepo := repository.NewUserRepository(pool)
	profileRepo := repository.NewProfileRepository(pool)

	uh := handler.NewUserHandler(userRepo)
	ph := handler.NewProfileHandler(profileRepo)
	prh := handler.NewPrefsHandler(profileRepo, rdb)
	photoh := handler.NewPhotoHandler(profileRepo, minioStore)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.RealIP)

	r.Get("/healthz", handler.HealthHandler)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/users", uh.CreateUser)
		r.Get("/users/{user_id}", uh.GetUser)

		r.Get("/profiles/{user_id}", ph.GetProfile)
		r.Put("/profiles/{user_id}", ph.UpdateProfile)

		r.Post("/profiles/{user_id}/photos", photoh.UploadPhoto)
		r.Get("/profiles/{user_id}/photos/primary/meta", photoh.GetPrimaryPhotoMeta)
		r.Get("/profiles/{user_id}/photos/primary", photoh.GetPrimaryPhoto)

		r.Put("/preferences/{user_id}", prh.UpdatePreferences)
	})

	log.Printf("profile-service listening on :%s", port)
	if err = http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server: %v", err)
	}
}
