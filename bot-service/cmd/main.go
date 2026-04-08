package main

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/handler"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
	"github.com/kurt4ins/drizzy/pkg/config"
	"github.com/redis/go-redis/v9"
	"context"
)

func main() {
	ctx := context.Background()

	token := config.Get("TELEGRAM_TOKEN")
	profileURL := config.Get("PROFILE_API_URL")
	redisAddr := config.Get("REDIS_ADDR")

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("telegram bot init: %v", err)
	}
	log.Printf("authorized as @%s", bot.Self.UserName)

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err = rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("ping redis: %v", err)
	}
	defer rdb.Close()

	ss := session.NewStore(rdb)
	pc := client.NewProfileClient(profileURL)
	startHandler := handler.NewStartHandler(bot, ss, pc)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	log.Println("bot-service started, waiting for updates...")
	for update := range updates {
		startHandler.Handle(ctx, update)
	}
}
