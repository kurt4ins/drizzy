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

func (h *StartHandler) HandleStart(ctx context.Context, msg *tgbotapi.Message) {
	_ = h.session.Del(ctx, msg.From.ID)
	if err := h.session.SetField(ctx, msg.From.ID, "step", stepAskName); err != nil {
		log.Printf("session set: %v", err)
		return
	}
	h.send(msg.Chat.ID, "Привет! Давай создадим твой профиль.\n\nКак тебя зовут?")
}

func (h *StartHandler) HandleTextInput(ctx context.Context, msg *tgbotapi.Message) {
	data, err := h.session.GetAll(ctx, msg.From.ID)
	if err != nil {
		log.Printf("session get: %v", err)
		return
	}

	step := data["step"]
	if step == "" {
		h.send(msg.Chat.ID, "Отправь /start для регистрации или /browse для просмотра анкет.")
		return
	}

	text := strings.TrimSpace(msg.Text)

	switch step {
	case stepAskName:
		if len(text) < 2 {
			h.send(msg.Chat.ID, "Имя должно содержать хотя бы 2 символа. Попробуй ещё раз:")
			return
		}
		h.advance(ctx, msg.From.ID, msg.Chat.ID, "name", text, stepAskAge, "Сколько тебе лет?")

	case stepAskAge:
		age, err := strconv.Atoi(text)
		if err != nil || age < 1 || age > 100 {
			h.send(msg.Chat.ID, "Введи корректный возраст (от 1 до 100):")
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "age", text)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskGender); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendGenderKeyboard(msg.Chat.ID)

	case stepAskGender:
		h.sendGenderKeyboard(msg.Chat.ID)

	case stepAskCity:
		if len(text) < 2 {
			h.send(msg.Chat.ID, "Укажи город (минимум 2 символа):")
			return
		}
		h.advance(ctx, msg.From.ID, msg.Chat.ID, "city", text, stepAskBio,
			"Расскажи немного о себе (или /skip):")

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
		userID, ok := h.saveProfileData(ctx, msg.From.ID, msg.From.UserName, msg.Chat.ID, data)
		if !ok {
			return
		}
		_ = h.session.SetField(ctx, msg.From.ID, "user_id", userID)
		if err = h.session.SetField(ctx, msg.From.ID, "step", stepAskPrefGender); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.sendPrefGenderKeyboard(msg.Chat.ID)

	case stepAskPrefGender:
		h.sendPrefGenderKeyboard(msg.Chat.ID)

	case stepAskPrefAge:
		h.handlePrefAge(ctx, msg.From.ID, msg.Chat.ID, text)

	case stepAskPhoto:
		h.send(msg.Chat.ID, "Пожалуйста, отправь фото (не файл, а именно фотографию), или /skip чтобы пропустить:")
		if text == "/skip" {
			h.finishRegistration(ctx, msg.From.ID, msg.Chat.ID, data["name"])
		}

	case StepEditName:
		if len(text) < 2 {
			h.send(msg.Chat.ID, "Имя должно содержать хотя бы 2 символа:")
			return
		}
		h.applyProfileEdit(ctx, msg.From.ID, msg.Chat.ID, func(p *models.UpdateProfileRequest) { p.Name = text })

	case StepEditBio:
		bio := text
		if bio == "/skip" {
			bio = ""
		}
		h.applyProfileEdit(ctx, msg.From.ID, msg.Chat.ID, func(p *models.UpdateProfileRequest) { p.Bio = bio })

	case StepEditInterests:
		var interests []string
		if text != "/skip" {
			interests = parseInterests(text)
		}
		h.applyProfileEdit(ctx, msg.From.ID, msg.Chat.ID, func(p *models.UpdateProfileRequest) { p.Interests = interests })

	case StepEditPhoto:
		h.send(msg.Chat.ID, "Отправь фото, а не текст:")
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
		h.uploadPhotoAndContinue(ctx, msg.From.ID, msg.Chat.ID, userID, best, func() {
			h.finishRegistration(ctx, msg.From.ID, msg.Chat.ID, data["name"])
		})

	case StepEditPhoto:
		userID, _ := h.userStore.GetUserID(ctx, msg.From.ID)
		if userID == "" {
			h.send(msg.Chat.ID, "Что-то пошло не так. Попробуй /start.")
			return
		}
		h.uploadPhotoAndContinue(ctx, msg.From.ID, msg.Chat.ID, userID, best, func() {
			_ = h.session.SetField(ctx, msg.From.ID, "step", "")
			h.send(msg.Chat.ID, "Фото обновлено! ✅")
			if h.myProfile != nil {
				h.myProfile.ShowProfileAfterEdit(ctx, msg.Chat.ID, userID)
			}
		})
	}
}

func (h *StartHandler) uploadPhotoAndContinue(
	ctx context.Context,
	telegramID int64, chatID int64,
	userID string,
	photo tgbotapi.PhotoSize,
	onSuccess func(),
) {
	fileBytes, err := h.downloadTelegramFile(photo.FileID)
	if err != nil {
		log.Printf("download telegram file: %v", err)
		h.send(chatID, "Не удалось загрузить фото. Попробуй ещё раз:")
		return
	}
	if _, err = h.profile.UploadPhoto(ctx, userID, photo.FileID, fileBytes); err != nil {
		log.Printf("upload photo: %v", err)
		h.send(chatID, "Не удалось сохранить фото. Попробуй ещё раз:")
		return
	}
	onSuccess()
}

func (h *StartHandler) HandleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	answer := tgbotapi.NewCallback(cb.ID, "")
	if _, err := h.bot.Request(answer); err != nil {
		log.Printf("answer callback: %v", err)
	}

	data, err := h.session.GetAll(ctx, cb.From.ID)
	if err != nil {
		log.Printf("session get: %v", err)
		return
	}

	switch data["step"] {
	case stepAskGender:
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

	case stepAskPrefGender:
		if !strings.HasPrefix(cb.Data, "pref:") {
			return
		}
		prefGender := strings.TrimPrefix(cb.Data, "pref:")
		_ = h.session.SetField(ctx, cb.From.ID, "pref_gender", prefGender)
		if err = h.session.SetField(ctx, cb.From.ID, "step", stepAskPrefAge); err != nil {
			log.Printf("session set: %v", err)
			return
		}
		h.send(cb.Message.Chat.ID,
			"Какой возраст тебя интересует?\nВведи диапазон, например *18-30*, или /skip")
	}
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

	if text != "/skip" {
		parts := strings.SplitN(text, "-", 2)
		if len(parts) != 2 {
			h.send(chatID, "Введи диапазон через дефис, например *20-30*, или /skip:")
			return
		}
		minAge, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		maxAge, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || minAge < 1 || maxAge > 100 || minAge > maxAge {
			h.send(chatID, "Некорректный диапазон. Например: *20-30*. Попробуй ещё раз или /skip:")
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
	h.send(chatID, "Отлично! Теперь добавь фото профиля.\nОтправь фотографию или /skip:")
}

func (h *StartHandler) savePreferences(ctx context.Context, userID string, data map[string]string) {
	req := models.UpdatePreferencesRequest{}

	switch data["pref_gender"] {
	case "male":
		req.PrefGender = []string{"male"}
	case "female":
		req.PrefGender = []string{"female"}
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

	h.send(chatID, fmt.Sprintf(
		"Готово, %s! Добро пожаловать в Drizzy 🎉\n\nТвой профиль создан. Отправь /browse чтобы смотреть анкеты.",
		name,
	))
}

func (h *StartHandler) applyProfileEdit(ctx context.Context, telegramID int64, chatID int64, mutate func(*models.UpdateProfileRequest)) {
	userID, err := h.userStore.GetUserID(ctx, telegramID)
	if err != nil || userID == "" {
		h.send(chatID, "Что-то пошло не так. Попробуй /start.")
		return
	}

	current, err := h.profile.GetProfile(ctx, userID)
	if err != nil {
		h.send(chatID, "Не удалось загрузить профиль. Попробуй позже.")
		return
	}

	req := models.UpdateProfileRequest{
		Name:      current.Name,
		Age:       current.Age,
		Gender:    current.Gender,
		City:      current.City,
		Bio:       current.Bio,
		Interests: current.Interests,
	}
	mutate(&req)

	if _, err = h.profile.UpdateProfile(ctx, userID, req); err != nil {
		log.Printf("update profile: %v", err)
		h.send(chatID, "Не удалось сохранить изменения. Попробуй позже.")
		return
	}

	_ = h.session.SetField(ctx, telegramID, "step", "")
	h.send(chatID, "Готово! ✅")
	if h.myProfile != nil {
		h.myProfile.ShowProfileAfterEdit(ctx, chatID, userID)
	}
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
	h.send(chatID, prompt)
}

func (h *StartHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
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

func (h *StartHandler) sendPrefGenderKeyboard(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Кого ты ищешь?")
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Парней", "pref:male"),
			tgbotapi.NewInlineKeyboardButtonData("Девушек", "pref:female"),
			tgbotapi.NewInlineKeyboardButtonData("Всех", "pref:any"),
		),
	)
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("send pref gender keyboard: %v", err)
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
