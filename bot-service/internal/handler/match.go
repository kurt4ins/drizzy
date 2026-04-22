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

type MatchConsumer struct {
	bot      *tgbotapi.BotAPI
	profile  *client.ProfileClient
	consumer *rabbitmq.Consumer
}

func NewMatchConsumer(bot *tgbotapi.BotAPI, profile *client.ProfileClient, rabbitURL string) (*MatchConsumer, error) {
	c, err := rabbitmq.NewConsumer(rabbitURL, rabbitmq.QueueMatchNotify, rabbitmq.RoutingKeyMatchCreated)
	if err != nil {
		return nil, fmt.Errorf("new match consumer: %w", err)
	}
	return &MatchConsumer{bot: bot, profile: profile, consumer: c}, nil
}

func (mc *MatchConsumer) Close() { mc.consumer.Close() }

func (mc *MatchConsumer) Run(ctx context.Context) error {
	log.Println("match consumer started")
	return mc.consumer.Consume(ctx, func(body []byte) error {
		return mc.handle(ctx, body)
	})
}

func (mc *MatchConsumer) handle(ctx context.Context, body []byte) error {
	var env events.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	if env.Type != events.TypeMatchCreated {
		return nil
	}

	payloadBytes, err := json.Marshal(env.Payload)
	if err != nil {
		return fmt.Errorf("re-marshal payload: %w", err)
	}
	var p events.MatchCreatedPayload
	if err = json.Unmarshal(payloadBytes, &p); err != nil {
		return fmt.Errorf("unmarshal match payload: %w", err)
	}

	pairs := [][2]string{{p.UserAID, p.UserBID}, {p.UserBID, p.UserAID}}
	for _, pair := range pairs {
		myUID, otherUID := pair[0], pair[1]

		myUser, err := mc.profile.GetUser(ctx, myUID)
		if err != nil {
			log.Printf("match consumer: get user %s: %v", myUID, err)
			continue
		}

		otherProfile, err := mc.profile.GetProfile(ctx, otherUID)
		if err != nil {
			log.Printf("match consumer: get profile %s: %v", otherUID, err)
			msg := tgbotapi.NewMessage(myUser.TelegramID,
				"🎉 У тебя новый мэтч! Кто-то тоже лайкнул тебя. Напиши им первым!")
			if _, sendErr := mc.bot.Send(msg); sendErr != nil {
				log.Printf("match consumer: send to %d: %v", myUser.TelegramID, sendErr)
			}
			continue
		}

		otherUser, err := mc.profile.GetUser(ctx, otherUID)
		if err != nil {
			log.Printf("match consumer: get user %s: %v", otherUID, err)
		}

		var contact string
		if otherUser.TelegramUsername != "" {
			contact = fmt.Sprintf("\n\n👤 [@%s](https://t.me/%s)",
				otherUser.TelegramUsername, otherUser.TelegramUsername)
		} else if otherUser.TelegramID != 0 {
			contact = fmt.Sprintf("\n\n👤 [Открыть профиль](tg://user?id=%d)", otherUser.TelegramID)
		}

		text := fmt.Sprintf(
			"🎉 Мэтч! *%s*, %d лет, %s тоже лайкнул(а) тебя!%s",
			escapeMarkdown(otherProfile.Name),
			otherProfile.Age,
			escapeMarkdown(otherProfile.City),
			contact,
		)
		msg := tgbotapi.NewMessage(myUser.TelegramID, text)
		msg.ParseMode = tgbotapi.ModeMarkdown
		if _, sendErr := mc.bot.Send(msg); sendErr != nil {
			log.Printf("match consumer: send to %d: %v", myUser.TelegramID, sendErr)
		}
	}
	return nil
}
