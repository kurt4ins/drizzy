package handler

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
	"github.com/kurt4ins/drizzy/bot-service/internal/userstore"
)

const (
	StepEditName      = "edit_name"
	StepEditBio       = "edit_bio"
	StepEditInterests = "edit_interests"
	StepEditPhoto     = "edit_photo"
)

type MyProfileHandler struct {
	bot       *tgbotapi.BotAPI
	profile   *client.ProfileClient
	session   *session.Store
	userStore *userstore.Store
}

func NewMyProfileHandler(
	bot *tgbotapi.BotAPI,
	pc *client.ProfileClient,
	ss *session.Store,
	us *userstore.Store,
) *MyProfileHandler {
	return &MyProfileHandler{bot: bot, profile: pc, session: ss, userStore: us}
}

func (h *MyProfileHandler) HandleMyProfile(ctx context.Context, msg *tgbotapi.Message) {
	userID, err := h.userStore.GetUserID(ctx, msg.From.ID)
	if err != nil || userID == "" {
		h.send(msg.Chat.ID, "Сначала зарегистрируйся — отправь /start.")
		return
	}
	h.showProfile(ctx, msg.Chat.ID, userID)
}

func (h *MyProfileHandler) HandleEditCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	answer := tgbotapi.NewCallback(cb.ID, "")
	if _, err := h.bot.Request(answer); err != nil {
		log.Printf("answer callback: %v", err)
	}

	userID, err := h.userStore.GetUserID(ctx, cb.From.ID)
	if err != nil || userID == "" {
		return
	}

	field := strings.TrimPrefix(cb.Data, "edit:")
	var step, prompt string
	switch field {
	case "name":
		step, prompt = StepEditName, "Введи новое имя:"
	case "bio":
		step, prompt = StepEditBio, "Напиши новое описание (или /skip чтобы очистить):"
	case "interests":
		step, prompt = StepEditInterests, "Укажи интересы через запятую (или /skip чтобы очистить):"
	case "photo":
		step, prompt = StepEditPhoto, "Отправь новое фото профиля:"
	default:
		return
	}

	if err = h.session.SetField(ctx, cb.From.ID, "step", step); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.send(cb.Message.Chat.ID, prompt)
}

func (h *MyProfileHandler) showProfile(ctx context.Context, chatID int64, userID string) {
	profile, err := h.profile.GetProfile(ctx, userID)
	if err != nil {
		log.Printf("myprofile: get profile %s: %v", userID, err)
		h.send(chatID, "Не удалось загрузить профиль. Попробуй позже.")
		return
	}

	interests := strings.Join(profile.Interests, ", ")
	if interests == "" {
		interests = "не указаны"
	}
	bio := profile.Bio
	if bio == "" {
		bio = "не заполнено"
	}

	text := fmt.Sprintf(
		"*Твой профиль*\n\n"+
			"👤 *%s*, %d лет\n"+
			"📍 %s\n\n"+
			"📝 %s\n\n"+
			"🎯 Интересы: %s\n\n"+
			"⭐ Заполненность: %.0f%%",
		escapeMarkdown(profile.Name),
		profile.Age,
		escapeMarkdown(profile.City),
		escapeMarkdown(bio),
		escapeMarkdown(interests),
		float64(profile.CompletenessScore)*100,
	)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Имя", "edit:name"),
			tgbotapi.NewInlineKeyboardButtonData("📝 Bio", "edit:bio"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🎯 Интересы", "edit:interests"),
			tgbotapi.NewInlineKeyboardButtonData("📷 Фото", "edit:photo"),
		),
	)

	photoFileID, _ := h.profile.GetPrimaryPhotoFileID(ctx, userID)
	if photoFileID != "" {
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(photoFileID))
		photoMsg.Caption = text
		photoMsg.ParseMode = tgbotapi.ModeMarkdown
		photoMsg.ReplyMarkup = keyboard
		if _, err = h.bot.Send(photoMsg); err != nil {
			log.Printf("myprofile: send photo: %v", err)
		}
		return
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard
	if _, err = h.bot.Send(msg); err != nil {
		log.Printf("myprofile: send: %v", err)
	}
}

func (h *MyProfileHandler) ShowProfileAfterEdit(ctx context.Context, chatID int64, userID string) {
	h.showProfile(ctx, chatID, userID)
}

func (h *MyProfileHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send: %v", err)
	}
}
