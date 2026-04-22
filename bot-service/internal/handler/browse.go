package handler

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/discovery"
	"github.com/kurt4ins/drizzy/bot-service/internal/userstore"
	"github.com/kurt4ins/drizzy/pkg/events"
	"github.com/kurt4ins/drizzy/pkg/rabbitmq"
)

type BrowseHandler struct {
	bot       *tgbotapi.BotAPI
	profile   *client.ProfileClient
	ranking   *client.RankingClient
	queue     *discovery.Queue
	users     *userstore.Store
	publisher *rabbitmq.Publisher
}

func NewBrowseHandler(
	bot *tgbotapi.BotAPI,
	profile *client.ProfileClient,
	ranking *client.RankingClient,
	queue *discovery.Queue,
	users *userstore.Store,
	pub *rabbitmq.Publisher,
) *BrowseHandler {
	return &BrowseHandler{bot: bot, profile: profile, ranking: ranking, queue: queue, users: users, publisher: pub}
}

func (h *BrowseHandler) HandleBrowseCommand(ctx context.Context, msg *tgbotapi.Message) {
	userID, err := h.users.GetUserID(ctx, msg.From.ID)
	if err != nil {
		log.Printf("browse: get user id: %v", err)
		h.send(msg.Chat.ID, "Произошла ошибка. Попробуй позже.")
		return
	}
	if userID == "" {
		h.send(msg.Chat.ID, "Сначала зарегистрируйся — отправь /start.")
		return
	}
	h.showNextCandidate(ctx, msg.Chat.ID, userID)
}

func (h *BrowseHandler) HandleInteractionCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	answer := tgbotapi.NewCallback(cb.ID, "")
	if _, err := h.bot.Request(answer); err != nil {
		log.Printf("answer callback: %v", err)
	}

	parts := strings.SplitN(cb.Data, ":", 2)
	if len(parts) != 2 {
		return
	}
	action, targetID := parts[0], parts[1]
	if action != "like" && action != "skip" {
		return
	}

	actorID, err := h.users.GetUserID(ctx, cb.From.ID)
	if err != nil || actorID == "" {
		log.Printf("browse callback: actor not found: %v", err)
		return
	}

	eventType := events.TypeInteractionSkipped
	if action == "like" {
		eventType = events.TypeInteractionLiked
	}

	env := events.Envelope{
		EventID:       uuid.New().String(),
		Type:          eventType,
		OccurredAt:    cb.Message.Time(),
		SchemaVersion: 1,
		Payload: events.InteractionPayload{
			ActorUserID:  actorID,
			TargetUserID: targetID,
		},
	}
	if err = h.publisher.Publish(ctx, eventType, env); err != nil {
		log.Printf("publish %s: %v", eventType, err)
	}

	emoji := "👎"
	if action == "like" {
		emoji = "❤️"
	}
	edit := tgbotapi.NewEditMessageReplyMarkup(
		cb.Message.Chat.ID,
		cb.Message.MessageID,
		tgbotapi.InlineKeyboardMarkup{InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{}},
	)
	if _, err = h.bot.Request(edit); err != nil {
		log.Printf("edit message: %v", err)
	}
	h.send(cb.Message.Chat.ID, fmt.Sprintf("%s Оценено! Отправь /browse для следующего.", emoji))
}

func (h *BrowseHandler) showNextCandidate(ctx context.Context, chatID int64, viewerUserID string) {
	candidateID, err := h.queue.Next(ctx, viewerUserID)
	if err != nil {
		log.Printf("browse: queue next: %v", err)
		h.send(chatID, "Что-то пошло не так. Попробуй позже.")
		return
	}

	if candidateID == "" && h.ranking != nil {
		if refillErr := h.ranking.RefillQueue(ctx, viewerUserID); refillErr != nil {
			log.Printf("browse: on-demand refill for %s: %v", viewerUserID, refillErr)
		} else {
			candidateID, err = h.queue.Next(ctx, viewerUserID)
			if err != nil {
				log.Printf("browse: queue next after refill: %v", err)
				h.send(chatID, "Что-то пошло не так. Попробуй позже.")
				return
			}
		}
	}

	if candidateID == "" {
		h.send(chatID, "Пока нет новых анкет. Загляни позже — скоро появятся! 🕐")
		return
	}

	profile, err := h.profile.GetProfile(ctx, candidateID)
	if err != nil {
		log.Printf("browse: get profile %s: %v", candidateID, err)
		h.send(chatID, "Не удалось загрузить анкету. Попробуй /browse снова.")
		return
	}

	interests := strings.Join(profile.Interests, ", ")
	if interests == "" {
		interests = "не указаны"
	}
	caption := fmt.Sprintf(
		"*%s*, %d лет, %s\n\n%s\n\n🎯 Интересы: %s",
		escapeMarkdown(profile.Name),
		profile.Age,
		escapeMarkdown(profile.City),
		escapeMarkdown(profile.Bio),
		escapeMarkdown(interests),
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❤️ Лайк", "like:"+candidateID),
			tgbotapi.NewInlineKeyboardButtonData("👎 Пропустить", "skip:"+candidateID),
		),
	)

	photoFileID, err := h.profile.GetPrimaryPhotoFileID(ctx, candidateID)
	if err != nil {
		log.Printf("browse: get photo file_id for %s: %v", candidateID, err)
	}

	if photoFileID != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(photoFileID))
		photoMsg.Caption = caption
		photoMsg.ParseMode = tgbotapi.ModeMarkdown
		photoMsg.ReplyMarkup = keyboard
		if _, err = h.bot.Send(photoMsg); err != nil {
			log.Printf("browse: send photo: %v", err)
		}
		return
	}

	msg := tgbotapi.NewMessage(chatID, caption)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	if _, err = h.bot.Send(msg); err != nil {
		log.Printf("browse: send: %v", err)
	}
}

func (h *BrowseHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}

func escapeMarkdown(s string) string {
	r := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return r.Replace(s)
}
