package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
	"github.com/kurt4ins/drizzy/pkg/models"
)

// Wizard step constants stored in session hash under field "step".
const (
	stepAskName      = "ask_name"
	stepAskAge       = "ask_age"
	stepAskGender    = "ask_gender"
	stepAskCity      = "ask_city"
	stepAskBio       = "ask_bio"
	stepAskInterests = "ask_interests"
)

type StartHandler struct {
	bot     *tgbotapi.BotAPI
	session *session.Store
	profile *client.ProfileClient
}

func NewStartHandler(bot *tgbotapi.BotAPI, ss *session.Store, pc *client.ProfileClient) *StartHandler {
	return &StartHandler{bot: bot, session: ss, profile: pc}
}

// Handle dispatches incoming Telegram updates.
func (h *StartHandler) Handle(ctx context.Context, update tgbotapi.Update) {
	switch {
	case update.Message != nil && update.Message.IsCommand():
		if update.Message.Command() == "start" {
			h.handleStart(ctx, update.Message)
		}

	case update.CallbackQuery != nil:
		h.handleCallback(ctx, update.CallbackQuery)

	case update.Message != nil:
		h.handleTextInput(ctx, update.Message)
	}
}

func (h *StartHandler) handleStart(ctx context.Context, msg *tgbotapi.Message) {
	_ = h.session.Del(ctx, msg.From.ID)
	if err := h.session.SetField(ctx, msg.From.ID, "step", stepAskName); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.send(msg.Chat.ID, "Привет! Давай создадим твой профиль.\n\nКак тебя зовут?")
}

func (h *StartHandler) handleTextInput(ctx context.Context, msg *tgbotapi.Message) {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil {
		log.Printf("session get: %v", err)
		return
	}

	step := data["step"]
	if step == "" {
		h.send(msg.Chat.ID, "Отправь /start чтобы начать регистрацию.")
		return
	}

	text := strings.TrimSpace(msg.Text)

	switch step {
	case stepAskName:
		if len(text) < 2 {
			h.send(msg.Chat.ID, "Имя должно содержать хотя бы 2 символа. Попробуй ещё раз:")
			return
		}
		h.advance(ctx, msg.From.ID, msg.Chat.ID, "name", text, stepAskAge,
			"Сколько тебе лет?")

	case stepAskAge:
		age, err := strconv.Atoi(text)
		if err != nil || age < 18 || age > 100 {
			h.send(msg.Chat.ID, "Введи корректный возраст (от 18 до 100):")
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "age", text)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskGender); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendGenderKeyboard(msg.Chat.ID)

	case stepAskGender:
		// User typed instead of tapping the button — re-send the keyboard.
		h.sendGenderKeyboard(msg.Chat.ID)

	case stepAskCity:
		if len(text) < 2 {
			h.send(msg.Chat.ID, "Укажи город (минимум 2 символа):")
			return
		}
		h.advance(ctx, msg.From.ID, msg.Chat.ID, "city", text, stepAskBio,
			"Расскажи немного о себе (или отправь /skip):")

	case stepAskBio:
		bio := ""
		if text != "/skip" {
			bio = text
		}
		_ = h.session.SetField(ctx, msg.From.ID, "bio", bio)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskInterests); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.send(msg.Chat.ID, "Какие у тебя интересы? Например: путешествия, музыка, спорт\n(или /skip)")

	case stepAskInterests:
		interests := ""
		if text != "/skip" {
			interests = text
		}
		_ = h.session.SetField(ctx, msg.From.ID, "interests", interests)
		data["interests"] = interests
		data["step"] = "done"
		h.completeRegistration(ctx, msg.From.ID, msg.From.UserName, msg.Chat.ID, data)
	}
}

func (h *StartHandler) handleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	// Answer the callback to remove the loading spinner.
	answer := tgbotapi.NewCallback(cb.ID, "")
	if _, err := h.bot.Request(answer); err != nil {
		log.Printf("answer callback: %v", err)
	}

	data, err := h.session.GetAll(ctx, cb.From.ID)
	if err != nil {
		log.Printf("session get: %v", err)
		return
	}

	if data["step"] != stepAskGender {
		return
	}

	gender := cb.Data
	if gender != "male" && gender != "female" {
		return
	}

	_ = h.session.SetField(ctx, cb.From.ID, "gender", gender)
	if err = h.session.SetField(ctx, cb.From.ID, "step", stepAskCity); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.send(cb.Message.Chat.ID, "В каком городе ты живёшь?")
}

func (h *StartHandler) completeRegistration(ctx context.Context, telegramID int64, username string, chatID int64, data map[string]string) {
	resp, err := h.profile.CreateUser(ctx, models.CreateUserRequest{
		TelegramID:       telegramID,
		TelegramUsername: username,
	})
	if err != nil {
		log.Printf("create user: %v", err)
		h.send(chatID, "Что-то пошло не так. Попробуй снова или отправь /start.")
		return
	}

	age, _ := strconv.Atoi(data["age"])
	interests := parseInterests(data["interests"])

	req := models.UpdateProfileRequest{
		Name:      data["name"],
		Age:       age,
		Gender:    data["gender"],
		City:      data["city"],
		Bio:       data["bio"],
		Interests: interests,
	}

	if _, err = h.profile.UpdateProfile(ctx, resp.User.ID, req); err != nil {
		log.Printf("update profile: %v", err)
		h.send(chatID, "Профиль создан, но не удалось сохранить данные. Попробуй /start снова.")
		return
	}

	_ = h.session.Del(ctx, telegramID)

	h.send(chatID, fmt.Sprintf(
		"Готово, %s! Добро пожаловать в Drizzy 🎉\n\nТвой профиль создан.",
		data["name"],
	))
}

// advance stores a field value, sets the next step, and sends a prompt.
func (h *StartHandler) advance(ctx context.Context, telegramID int64, chatID int64, field, value, nextStep, prompt string) {
	_ = h.session.SetField(ctx, telegramID, field, value)
	if err := h.session.SetField(ctx, telegramID, "step", nextStep); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.send(chatID, prompt)
}

func (h *StartHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send message: %v", err)
	}
}

func (h *StartHandler) sendGenderKeyboard(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Кто ты?")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Парень", "male"),
			tgbotapi.NewInlineKeyboardButtonData("Девушка", "female"),
		),
	)
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send gender keyboard: %v", err)
	}
}

// parseInterests splits a comma-separated string into a slice.
// Returns nil for empty input.
func parseInterests(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
