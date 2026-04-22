package main

import (
	"context"
	"log"
	"os/signal"
	"strings"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/discovery"
	"github.com/kurt4ins/drizzy/bot-service/internal/handler"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
	"github.com/kurt4ins/drizzy/bot-service/internal/userstore"
	"github.com/kurt4ins/drizzy/pkg/config"
	"github.com/kurt4ins/drizzy/pkg/rabbitmq"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	token := config.Get("TELEGRAM_TOKEN")
	profileURL := config.Get("PROFILE_API_URL")
	rankingURL := config.Get("RANKING_API_URL")
	redisAddr := config.Get("REDIS_ADDR")
	rabbitURL := config.Get("RABBITMQ_URL")

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

	pc := client.NewProfileClient(profileURL)
	rc := client.NewRankingClient(rankingURL)
	ss := session.NewStore(rdb)
	us := userstore.New(rdb)
	dq := discovery.New(rdb)

	pub, err := rabbitmq.NewPublisher(rabbitURL)
	if err != nil {
		log.Fatalf("rabbitmq publisher: %v", err)
	}
	defer pub.Close()

	matchConsumer, err := handler.NewMatchConsumer(bot, pc, rabbitURL)
	if err != nil {
		log.Fatalf("match consumer: %v", err)
	}
	defer matchConsumer.Close()

	likeConsumer, err := handler.NewLikeConsumer(bot, pc, rabbitURL)
	if err != nil {
		log.Fatalf("like consumer: %v", err)
	}
	defer likeConsumer.Close()

	startHandler := handler.NewStartHandler(bot, ss, pc, us)
	browseHandler := handler.NewBrowseHandler(bot, pc, rc, dq, us, pub)
	myProfileHandler := handler.NewMyProfileHandler(bot, pc, ss, us)

	startHandler.SetMyProfileHandler(myProfileHandler)

	registerCommands(bot)

	go func() {
		if err := matchConsumer.Run(ctx); err != nil {
			log.Printf("match consumer stopped: %v", err)
			stop()
		}
	}()

	go func() {
		if err := likeConsumer.Run(ctx); err != nil {
			log.Printf("like consumer stopped: %v", err)
			stop()
		}
	}()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	log.Println("bot-service started, waiting for updates...")
	for {
		select {
		case <-ctx.Done():
			log.Println("bot-service shutting down")
			return
		case update := <-updates:
			go dispatch(ctx, update, startHandler, browseHandler, myProfileHandler)
		}
	}
}

func dispatch(
	ctx context.Context,
	update tgbotapi.Update,
	start *handler.StartHandler,
	browse *handler.BrowseHandler,
	myprofile *handler.MyProfileHandler,
) {
	switch {
	case update.Message != nil && update.Message.IsCommand():
		switch update.Message.Command() {
		case "start":
			start.HandleStart(ctx, update.Message)
		case "browse":
			browse.HandleBrowseCommand(ctx, update.Message)
		case "profile":
			myprofile.HandleMyProfile(ctx, update.Message)
		default:
			start.HandleTextInput(ctx, update.Message)
		}

	case update.CallbackQuery != nil:
		data := update.CallbackQuery.Data
		switch {
		case strings.HasPrefix(data, "like:") || strings.HasPrefix(data, "skip:"):
			browse.HandleInteractionCallback(ctx, update.CallbackQuery)
		case strings.HasPrefix(data, "edit:"):
			myprofile.HandleEditCallback(ctx, update.CallbackQuery)
		default:
			start.HandleCallback(ctx, update.CallbackQuery)
		}

	case update.Message != nil && len(update.Message.Photo) > 0:
		start.HandlePhoto(ctx, update.Message)

	case update.Message != nil:
		start.HandleTextInput(ctx, update.Message)
	}
}

func registerCommands(bot *tgbotapi.BotAPI) {
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Регистрация / перезапуск"},
		tgbotapi.BotCommand{Command: "browse", Description: "Смотреть анкеты"},
		tgbotapi.BotCommand{Command: "profile", Description: "Мой профиль и редактирование"},
	)
	if _, err := bot.Request(commands); err != nil {
		log.Printf("set commands: %v", err)
	} else {
		log.Println("bot commands registered")
	}
}
