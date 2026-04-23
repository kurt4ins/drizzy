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

type MatchesHandler struct {
	bot     *tgbotapi.BotAPI
	profile *client.ProfileClient
	ranking *client.RankingClient
	users   *userstore.Store
	session *session.Store
}

func NewMatchesHandler(
	bot *tgbotapi.BotAPI,
	pc *client.ProfileClient,
	rc *client.RankingClient,
	us *userstore.Store,
	ss *session.Store,
) *MatchesHandler {
	return &MatchesHandler{bot: bot, profile: pc, ranking: rc, users: us, session: ss}
}

func (h *MatchesHandler) HandleMatches(ctx context.Context, msg *tgbotapi.Message) {
	userID, err := h.users.GetUserID(ctx, msg.From.ID)
	if err != nil {
		log.Printf("matches: get user id: %v", err)
		h.send(msg.Chat.ID, "Произошла ошибка. Попробуй позже.")
		return
	}
	if userID == "" {
		h.send(msg.Chat.ID, "Сначала зарегистрируйся — отправь /start.")
		return
	}
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyBrowseTarget, "")
	_ = h.session.SetField(ctx, msg.From.ID, SessionKeyProfileScreen, "")
	if h.ranking == nil {
		h.send(msg.Chat.ID, "Сервис мэтчей временно недоступен. Попробуй позже.")
		return
	}

	entries, err := h.ranking.ListMatches(ctx, userID)
	if err != nil {
		log.Printf("matches: list for %s: %v", userID, err)
		h.send(msg.Chat.ID, "Не удалось загрузить мэтчи. Попробуй позже.")
		return
	}

	if len(entries) == 0 {
		h.send(msg.Chat.ID, "Пока нет мэтчей. Ставь ❤️ в ленте — взаимные лайки появятся здесь!")
		return
	}

	var b strings.Builder
	b.WriteString("*Твои мэтчи*\n\n")
	for _, e := range entries {
		p, err := h.profile.GetProfile(ctx, e.OtherUserID)
		if err != nil {
			log.Printf("matches: profile %s: %v", e.OtherUserID, err)
			b.WriteString("• не удалось загрузить профиль\n\n")
			continue
		}
		b.WriteString(fmt.Sprintf(
			"• *%s*, %d, %s\n  _%s_\n\n",
			EscapeMarkdown(p.Name),
			p.Age,
			EscapeMarkdown(p.City),
			e.MatchedAt.Format("02.01.2006 15:04"),
		))
	}

	m := tgbotapi.NewMessage(msg.Chat.ID, b.String())
	m.ParseMode = tgbotapi.ModeMarkdown
	if _, err := h.bot.Send(m); err != nil {
		log.Printf("matches: send: %v", err)
	}
}

func (h *MatchesHandler) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("matches send: %v", err)
	}
}
