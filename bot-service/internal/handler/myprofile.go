package handler

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/keyboard"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
	"github.com/kurt4ins/drizzy/bot-service/internal/userstore"
)

const SessionKeyProfileScreen = "profile_screen"

type MyProfileHandler struct {
	bot       *tgbotapi.BotAPI
	profile   *client.ProfileClient
	session   *session.Store
	userStore *userstore.Store
	start     *StartHandler
}

func NewMyProfileHandler(
	bot *tgbotapi.BotAPI,
	pc *client.ProfileClient,
	ss *session.Store,
	us *userstore.Store,
) *MyProfileHandler {
	return &MyProfileHandler{bot: bot, profile: pc, session: ss, userStore: us}
}

func (h *MyProfileHandler) SetStartHandler(s *StartHandler) {
	h.start = s
}

func (h *MyProfileHandler) HandleMyProfile(ctx context.Context, msg *tgbotapi.Message) {
	userID, err := h.userStore.GetUserID(ctx, msg.From.ID)
	if err != nil || userID == "" {
		h.send(msg.Chat.ID, "Сначала зарегистрируйся — отправь /start.")
		return
	}
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyBrowseTarget, "")
	h.showProfile(ctx, msg.From.ID, msg.Chat.ID, userID)
}

// HandleProfileScreenReply — «Изменить профиль» / «Назад» на экране просмотра профиля.
func (h *MyProfileHandler) HandleProfileScreenReply(ctx context.Context, msg *tgbotapi.Message) bool {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil || data["step"] != "" || data[SessionKeyProfileScreen] != "1" {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	switch text {
	case keyboard.BtnBack:
		_ = h.session.SetField(ctx, msg.From.ID, SessionKeyProfileScreen, "")
		h.sendWithReplyKeyboard(msg.Chat.ID, "Главное меню", keyboard.MainMenu())
		return true
	case keyboard.BtnEditProfileFull:
		_ = h.session.SetField(ctx, msg.From.ID, SessionKeyProfileScreen, "")
		if h.start != nil {
			h.start.BeginProfileRefill(ctx, msg)
		}
		return true
	default:
		return false
	}
}

func (h *MyProfileHandler) showProfile(ctx context.Context, telegramID, chatID int64, userID string) {
	profile, err := h.profile.GetProfile(ctx, userID)
	if err != nil {
		log.Printf("myprofile: get profile %s: %v", userID, err)
		h.send(chatID, "Не удалось загрузить профиль. Попробуй позже.")
		return
	}

	interests := strings.Join(profile.Interests, ", ")
	bio := strings.TrimSpace(profile.Bio)

	var b strings.Builder
	b.WriteString("*Твой профиль*\n\n")
	b.WriteString(fmt.Sprintf("👤 *%s*, %d лет\n", EscapeMarkdown(profile.Name), profile.Age))
	b.WriteString(fmt.Sprintf("📍 %s", EscapeMarkdown(profile.City)))
	if bio != "" {
		b.WriteString("\n\n📝 ")
		b.WriteString(EscapeMarkdown(bio))
	}
	if interests != "" {
		b.WriteString("\n\n🎯 Интересы: ")
		b.WriteString(EscapeMarkdown(interests))
	}
	text := b.String()

	replyKb := keyboard.ProfileScreenMenu()
	_ = h.session.SetField(ctx, telegramID, SessionKeyProfileScreen, "1")

	photoFileID, _ := h.profile.GetPrimaryPhotoFileID(ctx, userID)
	if photoFileID != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(photoFileID))
		photoMsg.Caption = text
		photoMsg.ParseMode = tgbotapi.ModeMarkdown
		photoMsg.ReplyMarkup = replyKb
		if _, err = h.bot.Send(photoMsg); err != nil {
			log.Printf("myprofile: send photo: %v", err)
		}
		return
	}

	m := tgbotapi.NewMessage(chatID, text)
	m.ParseMode = tgbotapi.ModeMarkdown
	m.ReplyMarkup = replyKb
	if _, err = h.bot.Send(m); err != nil {
		log.Printf("myprofile: send: %v", err)
	}
}

func (h *MyProfileHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}

func (h *MyProfileHandler) sendWithReplyKeyboard(chatID int64, text string, markup tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = markup
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}
