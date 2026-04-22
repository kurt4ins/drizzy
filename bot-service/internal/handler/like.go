package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/pkg/events"
	"github.com/kurt4ins/drizzy/pkg/rabbitmq"
)

type LikeConsumer struct {
	bot      *tgbotapi.BotAPI
	profile  *client.ProfileClient
	consumer *rabbitmq.Consumer
}

func NewLikeConsumer(bot *tgbotapi.BotAPI, profile *client.ProfileClient, rabbitURL string) (*LikeConsumer, error) {
	c, err := rabbitmq.NewConsumer(rabbitURL, rabbitmq.QueueLikeNotify, rabbitmq.RoutingKeyLikeReceived)
	if err != nil {
		return nil, fmt.Errorf("new like consumer: %w", err)
	}
	return &LikeConsumer{bot: bot, profile: profile, consumer: c}, nil
}

func (lc *LikeConsumer) Close() { lc.consumer.Close() }

func (lc *LikeConsumer) Run(ctx context.Context) error {
	log.Println("like consumer started")
	return lc.consumer.Consume(ctx, func(body []byte) error {
		return lc.handle(ctx, body)
	})
}

func (lc *LikeConsumer) handle(ctx context.Context, body []byte) error {
	var env events.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.Type != events.TypeLikeReceived {
		return nil
	}

	payloadBytes, err := json.Marshal(env.Payload)
	if err != nil {
		return fmt.Errorf("re-marshal payload: %w", err)
	}
	var p events.LikeReceivedPayload
	if err = json.Unmarshal(payloadBytes, &p); err != nil {
		return fmt.Errorf("unmarshal like payload: %w", err)
	}

	user, err := lc.profile.GetUser(ctx, p.TargetUserID)
	if err != nil {
		log.Printf("like consumer: get user %s: %v", p.TargetUserID, err)
		return nil
	}

	msg := tgbotapi.NewMessage(user.TelegramID,
		"💛 Кто-то лайкнул твою анкету! Открой /browse — вдруг это взаимно?")
	if _, err = lc.bot.Send(msg); err != nil {
		log.Printf("like consumer: send to %d: %v", user.TelegramID, err)
	}
	return nil
}
