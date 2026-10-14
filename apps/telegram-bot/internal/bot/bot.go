package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

// API is the part of the Telegram client the bot uses to reply.
type API interface {
	SendMessage(ctx context.Context, m telegram.SendMessage) (telegram.Message, error)
}

// Bot handles updates.
type Bot struct {
	API API
	Log *slog.Logger
}

const welcome = "Namaste! I'm Satark.\n\n" +
	"Forward me a suspicious SMS or message, or send a screenshot of a QR code, " +
	"and I'll tell you how risky it looks and what to do.\n\n" +
	"I will never ask for your OTP, UPI PIN or password."

// Handle implements Handler.
func (b *Bot) Handle(ctx context.Context, u telegram.Update) {
	if u.Message == nil {
		return
	}
	m := u.Message
	if m.Chat.Type != "private" {
		return // the bot only talks in private chats
	}
	cmd, _ := command(m.Text)
	switch cmd {
	case "/start", "/help":
		b.send(ctx, m.Chat.ID, welcome, nil)
	}
}

// command returns the bot command (lower-cased, without @botname) and its argument.
func command(text string) (string, string) {
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}
	cmd, arg, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(arg)
}

func (b *Bot) send(ctx context.Context, chatID int64, text string, kb *telegram.InlineKeyboardMarkup) {
	_, err := b.API.SendMessage(ctx, telegram.SendMessage{
		ChatID:             chatID,
		Text:               text,
		ReplyMarkup:        kb,
		LinkPreviewOptions: &telegram.LinkPreviewOptions{IsDisabled: true},
	})
	if err != nil {
		b.Log.Warn("sendMessage failed", "err", err)
	}
}
