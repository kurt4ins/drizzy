package handler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/kurt4ins/drizzy/bot-service/internal/client"
	"github.com/kurt4ins/drizzy/bot-service/internal/keyboard"
	"github.com/kurt4ins/drizzy/bot-service/internal/session"
	"github.com/kurt4ins/drizzy/bot-service/internal/userstore"
	"github.com/kurt4ins/drizzy/pkg/models"
)

const (
	stepAskName       = "ask_name"
	stepAskAge        = "ask_age"
	stepAskGender     = "ask_gender"
	stepAskCity       = "ask_city"
	stepAskBio        = "ask_bio"
	stepAskInterests  = "ask_interests"
	stepAskPrefGender = "ask_pref_gender"
	stepAskPrefAge    = "ask_pref_age"
	stepAskPhoto      = "ask_photo"

	sessionKeyProfileRefill = "profile_refill"
)

type StartHandler struct {
	bot       *tgbotapi.BotAPI
	session   *session.Store
	profile   *client.ProfileClient
	userStore *userstore.Store
	myProfile *MyProfileHandler
}

func NewStartHandler(bot *tgbotapi.BotAPI, ss *session.Store, pc *client.ProfileClient, us *userstore.Store) *StartHandler {
	return &StartHandler{bot: bot, session: ss, profile: pc, userStore: us}
}

func (h *StartHandler) SetMyProfileHandler(mph *MyProfileHandler) {
	h.myProfile = mph
}

// BeginProfileRefill запускает заново тот же сценарий, что и регистрация, для уже существующего пользователя.
func (h *StartHandler) BeginProfileRefill(ctx context.Context, msg *tgbotapi.Message) {
	userID, err := h.userStore.GetUserID(ctx, msg.From.ID)
	if err != nil || userID == "" {
		h.send(msg.Chat.ID, "Сначала зарегистрируйся — отправь /start.")
		return
	}
	_ = h.session.Del(ctx, msg.From.ID)
	_ = h.session.SetField(ctx, msg.From.ID, sessionKeyProfileRefill, "1")
	_ = h.session.SetField(ctx, msg.From.ID, "user_id", userID)
	if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskName); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Обновим анкету — пройди шаги как при регистрации.\n\nКак тебя зовут?")
	m.ReplyMarkup = keyboard.Remove()
	if _, err := h.bot.Send(m); err != nil {
		log.Printf("send message: %v", err)
	}
}

func (h *StartHandler) HandleStart(ctx context.Context, msg *tgbotapi.Message) {
	_ = h.session.Del(ctx, msg.From.ID)
	if err := h.session.SetField(ctx, msg.From.ID, "step", stepAskName); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	m := tgbotapi.NewMessage(msg.Chat.ID, "Привет! Давай создадим твой профиль.\n\nКак тебя зовут?")
	m.ReplyMarkup = keyboard.Remove()
	if _, err := h.bot.Send(m); err != nil {
		log.Printf("send message: %v", err)
	}
}

func (h *StartHandler) HandleTextInput(ctx context.Context, msg *tgbotapi.Message) {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil {
		log.Printf("session get: %v", err)
		return
	}

	step := data["step"]
	if step == "" {
		h.send(msg.Chat.ID,
			"Используй кнопки снизу или команды /browse, /profile, /matches и /start.")
		return
	}

	text := strings.TrimSpace(msg.Text)

	switch step {
	case stepAskName:
		if len(text) < 2 {
			h.sendRemoveReplyKeyboard(msg.Chat.ID, "Имя должно содержать хотя бы 2 символа. Попробуй ещё раз:")
			return
		}
		h.advance(ctx, msg.From.ID, msg.Chat.ID, "name", text, stepAskAge, "Сколько тебе лет?")

	case stepAskAge:
		age, err := strconv.Atoi(text)
		if err != nil || age < 1 || age > 100 {
			h.sendRemoveReplyKeyboard(msg.Chat.ID, "Введи корректный возраст (от 1 до 100):")
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "age", text)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskGender); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendWithReplyKeyboard(msg.Chat.ID, "Кто ты? Нажми вариант ниже 👇", keyboard.GenderSelf())

	case stepAskGender:
		var gender string
		switch text {
		case keyboard.BtnGenderMale:
			gender = "male"
		case keyboard.BtnGenderFemale:
			gender = "female"
		default:
			h.sendWithReplyKeyboard(msg.Chat.ID, "Выбери, кто ты, кнопкой ниже 👇", keyboard.GenderSelf())
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "gender", gender)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskCity); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendRemoveReplyKeyboard(msg.Chat.ID, "В каком городе ты живёшь?")

	case stepAskCity:
		if len(text) < 2 {
			h.sendRemoveReplyKeyboard(msg.Chat.ID, "Укажи город (минимум 2 символа):")
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "city", text)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskBio); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendWithReplyKeyboard(msg.Chat.ID,
			"Расскажи немного о себе. Можно нажать «Пропустить», если не хочешь заполнять.",
			keyboard.SkipOnly())

	case stepAskBio:
		bio := ""
		if text != "/skip" && text != keyboard.BtnSkip {
			bio = text
		}
		_ = h.session.SetField(ctx, msg.From.ID, "bio", bio)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskInterests); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendWithReplyKeyboard(msg.Chat.ID,
			"Какие у тебя интересы? Например: путешествия, музыка, спорт\n"+
				"Или нажми «Пропустить», если не хочешь указывать.",
			keyboard.SkipOnly())

	case stepAskInterests:
		interests := ""
		if text != "/skip" && text != keyboard.BtnSkip {
			interests = text
		}
		_ = h.session.SetField(ctx, msg.From.ID, "interests", interests)

		data["interests"] = interests
		if data[sessionKeyProfileRefill] == "1" {
			userID := data["user_id"]
			if userID == "" {
				h.send(msg.Chat.ID, "Что-то пошло не так. Попробуй /start.")
				return
			}
			age, _ := strconv.Atoi(data["age"])
			req := models.UpdateProfileRequest{
				Name:      data["name"],
				Age:       age,
				Gender:    data["gender"],
				City:      data["city"],
				Bio:       data["bio"],
				Interests: parseInterests(data["interests"]),
			}
			if _, err := h.profile.UpdateProfile(ctx, userID, req); err != nil {
				log.Printf("update profile (refill): %v", err)
				h.send(msg.Chat.ID, "Не удалось сохранить профиль. Попробуй позже.")
				return
			}
		} else {
			userID, ok := h.saveProfileData(ctx, msg.From.ID, msg.From.UserName, msg.Chat.ID, data)
			if !ok {
				return
			}
			_ = h.session.SetField(ctx, msg.From.ID, "user_id", userID)
		}
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskPrefGender); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendWithReplyKeyboard(msg.Chat.ID, "Кого ты ищешь? Нажми вариант ниже 👇", keyboard.PrefGender())

	case stepAskPrefGender:
		var pref string
		switch text {
		case keyboard.BtnPrefMale:
			pref = "male"
		case keyboard.BtnPrefFemale:
			pref = "female"
		case keyboard.BtnPrefAny:
			pref = "any"
		default:
			h.sendWithReplyKeyboard(msg.Chat.ID, "Выбери, кого ищешь, кнопкой ниже 👇", keyboard.PrefGender())
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "pref_gender", pref)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskPrefAge); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendWithReplyKeyboard(msg.Chat.ID,
			"Какой возраст тебя интересует?\nВведи диапазон, например *18-30*, или нажми «Пропустить».",
			keyboard.SkipOnly())

	case stepAskPrefAge:
		h.handlePrefAge(ctx, msg.From.ID, msg.Chat.ID, text)

	case stepAskPhoto:
		if text == "/skip" || text == keyboard.BtnSkip {
			if data[sessionKeyProfileRefill] == "1" {
				h.finishProfileRefill(ctx, msg.From.ID, msg.Chat.ID, data["name"])
			} else {
				h.finishRegistration(ctx, msg.From.ID, msg.Chat.ID, data["name"])
			}
			return
		}
		h.sendWithReplyKeyboard(msg.Chat.ID,
			"Пожалуйста, отправь фото (не файл, а именно фотографию), или нажми «Пропустить».",
			keyboard.SkipOnly())
	}
}

func (h *StartHandler) HandlePhoto(ctx context.Context, msg *tgbotapi.Message) {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil {
		log.Printf("session get: %v", err)
		return
	}

	photos := msg.Photo
	if len(photos) == 0 {
		return
	}
	best := photos[len(photos)-1]

	switch data["step"] {
	case stepAskPhoto:
		userID := data["user_id"]
		if userID == "" {
			h.send(msg.Chat.ID, "Что-то пошло не так. Попробуй /start.")
			return
		}
		refill := data[sessionKeyProfileRefill] == "1"
		name := data["name"]
		h.uploadPhotoAndContinue(ctx, msg.From.ID, msg.Chat.ID, userID, best, true, func() {
			if refill {
				h.finishProfileRefill(ctx, msg.From.ID, msg.Chat.ID, name)
			} else {
				h.finishRegistration(ctx, msg.From.ID, msg.Chat.ID, name)
			}
		})
	}
}

func (h *StartHandler) uploadPhotoAndContinue(
	ctx context.Context,
	telegramID int64, chatID int64,
	userID string,
	photo tgbotapi.PhotoSize,
	reattachSkipOnError bool,
	onSuccess func(),
) {
	fileBytes, err := h.downloadTelegramFile(photo.FileID)
	if err != nil {
		log.Printf("download telegram file: %v", err)
		h.photoUploadErrorReply(chatID, "Не удалось загрузить фото. Попробуй ещё раз:", reattachSkipOnError)
		return
	}
	if _, err = h.profile.UploadPhoto(ctx, userID, photo.FileID, fileBytes); err != nil {
		log.Printf("upload photo: %v", err)
		h.photoUploadErrorReply(chatID, "Не удалось сохранить фото. Попробуй ещё раз:", reattachSkipOnError)
		return
	}
	onSuccess()
}

func (h *StartHandler) photoUploadErrorReply(chatID int64, text string, reattachSkip bool) {
	if reattachSkip {
		h.sendWithReplyKeyboard(chatID, text, keyboard.SkipOnly())
		return
	}
	h.sendRemoveReplyKeyboard(chatID, text)
}

func (h *StartHandler) saveProfileData(ctx context.Context, telegramID int64, username string, chatID int64, data map[string]string) (string, bool) {
	resp, err := h.profile.CreateUser(ctx, models.CreateUserRequest{
		TelegramID:       telegramID,
		TelegramUsername: username,
	})
	if err != nil {
		log.Printf("create user: %v", err)
		h.send(chatID, "Что-то пошло не так. Попробуй /start снова.")
		return "", false
	}

	age, _ := strconv.Atoi(data["age"])
	req := models.UpdateProfileRequest{
		Name:      data["name"],
		Age:       age,
		Gender:    data["gender"],
		City:      data["city"],
		Bio:       data["bio"],
		Interests: parseInterests(data["interests"]),
	}
	if _, err = h.profile.UpdateProfile(ctx, resp.User.ID, req); err != nil {
		log.Printf("update profile: %v", err)
		h.send(chatID, "Не удалось сохранить профиль. Попробуй /start снова.")
		return "", false
	}
	return resp.User.ID, true
}

func (h *StartHandler) handlePrefAge(ctx context.Context, telegramID int64, chatID int64, text string) {
	data, _ := h.session.GetAll(ctx, telegramID)
	userID := data["user_id"]

	if text != "/skip" && text != keyboard.BtnSkip {
		parts := strings.SplitN(text, "-", 2)
		if len(parts) != 2 {
			h.sendWithReplyKeyboard(chatID,
				"Введи диапазон через дефис, например *20-30*, или нажми «Пропустить».",
				keyboard.SkipOnly())
			return
		}
		minAge, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxAge, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || minAge < 1 || maxAge > 100 || minAge > maxAge {
			h.sendWithReplyKeyboard(chatID,
				"Некорректный диапазон. Например: *20-30*. Попробуй ещё раз или «Пропустить».",
				keyboard.SkipOnly())
			return
		}
		_ = h.session.SetField(ctx, telegramID, "pref_age_min", strconv.Itoa(minAge))
		_ = h.session.SetField(ctx, telegramID, "pref_age_max", strconv.Itoa(maxAge))
		data["pref_age_min"] = strconv.Itoa(minAge)
		data["pref_age_max"] = strconv.Itoa(maxAge)
	}

	if userID != "" {
		h.savePreferences(ctx, userID, data)
	}

	if err := h.session.SetField(ctx, telegramID, "step", stepAskPhoto); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.sendWithReplyKeyboard(chatID,
		"Отлично! Теперь добавь фото профиля.\nОтправь фотографию или нажми «Пропустить».",
		keyboard.SkipOnly())
}

func (h *StartHandler) savePreferences(ctx context.Context, userID string, data map[string]string) {
	req := models.UpdatePreferencesRequest{}

	switch data["pref_gender"] {
	case "male":
		req.PrefGender = []string{"male"}
	case "female":
		req.PrefGender = []string{"female"}
	case "any":
		req.PrefGender = []string{}
	}

	if minStr := data["pref_age_min"]; minStr != "" {
		v, _ := strconv.Atoi(minStr)
		req.PrefAgeMin = &v
	}
	if maxStr := data["pref_age_max"]; maxStr != "" {
		v, _ := strconv.Atoi(maxStr)
		req.PrefAgeMax = &v
	}

	if _, err := h.profile.UpdatePreferences(ctx, userID, req); err != nil {
		log.Printf("update preferences: %v", err)
	}
}

func (h *StartHandler) finishRegistration(ctx context.Context, telegramID int64, chatID int64, name string) {
	data, _ := h.session.GetAll(ctx, telegramID)
	userID := data["user_id"]

	_ = h.session.Del(ctx, telegramID)

	if err := h.userStore.Save(ctx, telegramID, userID); err != nil {
		log.Printf("save user store: %v", err)
	}

	text := fmt.Sprintf(
		"Готово, *%s*! Добро пожаловать в Drizzy 🎉\n\n"+
			"Твой профиль создан.\n\n"+
			"*Кнопки снизу:*\n"+
			"• *%s* — смотреть анкеты\n"+
			"• *%s* — профиль и редактирование\n"+
			"• *%s* — взаимные лайки",
		EscapeMarkdown(name),
		EscapeMarkdown(keyboard.BtnBrowse),
		EscapeMarkdown(keyboard.BtnProfile),
		EscapeMarkdown(keyboard.BtnMatches),
	)
	h.sendWithReplyKeyboard(chatID, text, keyboard.MainMenu())
}

func (h *StartHandler) finishProfileRefill(ctx context.Context, telegramID int64, chatID int64, name string) {
	_ = h.session.Del(ctx, telegramID)
	text := fmt.Sprintf(
		"Готово, *%s*! Анкета обновлена.",
		EscapeMarkdown(name),
	)
	h.sendWithReplyKeyboard(chatID, text, keyboard.MainMenu())
}

func (h *StartHandler) downloadTelegramFile(fileID string) ([]byte, error) {
	url, err := h.bot.GetFileDirectURL(fileID)
	if err != nil {
		return nil, fmt.Errorf("get direct url: %w", err)
	}
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (h *StartHandler) advance(ctx context.Context, telegramID int64, chatID int64, field, value, nextStep, prompt string) {
	_ = h.session.SetField(ctx, telegramID, field, value)
	if err := h.session.SetField(ctx, telegramID, "step", nextStep); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.sendRemoveReplyKeyboard(chatID, prompt)
}

func (h *StartHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send message: %v", err)
	}
}

func (h *StartHandler) sendWithReplyKeyboard(chatID int64, text string, markup tgbotapi.ReplyKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = markup
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send message: %v", err)
	}
}

func (h *StartHandler) sendRemoveReplyKeyboard(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard.Remove()
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send message: %v", err)
	}
}

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
