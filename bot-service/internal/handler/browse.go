package handler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/discovery"
	"github.com/kurt4ins/drizzy/bot-service/internal/keyboard"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
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
	session   *session.Store
	publisher *rabbitmq.Publisher
}

func NewBrowseHandler(
	bot *tgbotapi.BotAPI,
	profile *client.ProfileClient,
	ranking *client.RankingClient,
	queue *discovery.Queue,
	users *userstore.Store,
	ss *session.Store,
	pub *rabbitmq.Publisher,
) *BrowseHandler {
	return &BrowseHandler{
		bot: bot, profile: profile, ranking: ranking, queue: queue,
		users: users, session: ss, publisher: pub,
	}
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
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyBrowseTarget, "")
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyProfileScreen, "")
	h.showNextCandidate(ctx, msg.Chat.ID, msg.From.ID, userID)
}

// HandleBrowseVote обрабатывает «❤️ Лайк» / «👎 Пропустить» по текущей анкете.
func (h *BrowseHandler) HandleBrowseVote(ctx context.Context, msg *tgbotapi.Message) bool {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil || data["step"] != "" {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	var action string
	switch text {
	case keyboard.BtnLike:
		action = "like"
	case keyboard.BtnBrowseSkip:
		action = "skip"
	default:
		return false
	}
	targetID := data[SessionKeyBrowseTarget]
	if targetID == "" {
		h.send(msg.Chat.ID, "Сначала открой ленту — «"+keyboard.BtnBrowse+"».")
		return true
	}
	if err := h.publishInteraction(ctx, msg, action, targetID); err != nil {
		log.Printf("browse vote: %v", err)
		h.send(msg.Chat.ID, "Не удалось сохранить оценку. Попробуй ещё раз.")
		return true
	}
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyBrowseTarget, "")

	actorID, err := h.users.GetUserID(ctx, msg.From.ID)
	if err != nil || actorID == "" {
		log.Printf("browse vote: actor after publish: %v", err)
		return true
	}
	h.showNextCandidate(ctx, msg.Chat.ID, msg.From.ID, actorID)
	return true
}

func (h *BrowseHandler) publishInteraction(ctx context.Context, msg *tgbotapi.Message, action, targetID string) error {
	actorID, err := h.users.GetUserID(ctx, msg.From.ID)
	if err != nil || actorID == "" {
		return fmt.Errorf("actor not found")
	}
	eventType := events.TypeInteractionSkipped
	if action == "like" {
		eventType = events.TypeInteractionLiked
	}
	env := events.Envelope{
		EventID:       uuid.New().String(),
		Type:          eventType,
		OccurredAt:    time.Now().UTC(),
		SchemaVersion: 1,
		Payload: events.InteractionPayload{
			ActorUserID:  actorID,
			TargetUserID: targetID,
		},
	}
	return h.publisher.Publish(ctx, eventType, env)
}

// HandleBrowseBack — «Назад» из ленты: сброс текущей анкеты и главное меню.
func (h *BrowseHandler) HandleBrowseBack(ctx context.Context, msg *tgbotapi.Message) bool {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil || data["step"] != "" {
		return false
	}
	if strings.TrimSpace(msg.Text) != keyboard.BtnBack {
		return false
	}
	if data[SessionKeyBrowseTarget] == "" {
		return false
	}
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyBrowseTarget, "")
	h.sendWithReplyKeyboard(msg.Chat.ID, "Главное меню", keyboard.MainMenu())
	return true
}

func (h *BrowseHandler) showNextCandidate(ctx context.Context, chatID, telegramID int64, viewerUserID string) {
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
		_ = h.session.SetField(ctx, telegramID, SessionKeyBrowseTarget, "")
		h.sendWithReplyKeyboard(chatID, "Пока нет новых анкет. Загляни позже — скоро появятся! 🕐", keyboard.MainMenu())
		return
	}

	if err = h.session.SetField(ctx, telegramID, SessionKeyBrowseTarget, candidateID); err != nil {
		log.Printf("browse: session set target: %v", err)
	}

	profile, err := h.profile.GetProfile(ctx, candidateID)
	if err != nil {
		log.Printf("browse: get profile %s: %v", candidateID, err)
		_ = h.session.SetField(ctx, telegramID, SessionKeyBrowseTarget, "")
		h.send(chatID, "Не удалось загрузить анкету. Попробуй /browse снова.")
		return
	}

	bio := strings.TrimSpace(profile.Bio)
	interests := strings.Join(profile.Interests, ", ")

	var b strings.Builder
	b.WriteString(fmt.Sprintf("*%s*, %d лет, %s", EscapeMarkdown(profile.Name), profile.Age, EscapeMarkdown(profile.City)))
	if bio != "" {
		b.WriteString("\n\n")
		b.WriteString(EscapeMarkdown(bio))
	}
	if interests != "" {
		b.WriteString("\n\n🎯 Интересы: ")
		b.WriteString(EscapeMarkdown(interests))
	}
	caption := b.String()

	replyKb := keyboard.BrowseVote()

	photoFileID, err := h.profile.GetPrimaryPhotoFileID(ctx, candidateID)
	if err != nil {
		log.Printf("browse: get photo file_id for %s: %v", candidateID, err)
	}

	if photoFileID != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(photoFileID))
		photoMsg.Caption = caption
		photoMsg.ParseMode = tgbotapi.ModeMarkdown
		photoMsg.ReplyMarkup = replyKb
		if _, err = h.bot.Send(photoMsg); err != nil {
			log.Printf("browse: send photo: %v", err)
		}
		return
	}

	m := tgbotapi.NewMessage(chatID, caption)
	m.ParseMode = tgbotapi.ModeMarkdown
	m.ReplyMarkup = replyKb
	if _, err = h.bot.Send(m); err != nil {
		log.Printf("browse: send: %v", err)
	}
}

func (h *BrowseHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}

func (h *BrowseHandler) sendWithReplyKeyboard(chatID int64, text string, markup tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = markup
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}
