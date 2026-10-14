package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/gateway"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

var scansTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "satark_telegram_scans_total",
	Help: "Scans requested through the bot, by kind and outcome.",
}, []string{"kind", "outcome"})

// API is the part of the Telegram client the bot uses.
type API interface {
	SendMessage(ctx context.Context, m telegram.SendMessage) (telegram.Message, error)
	DownloadFile(ctx context.Context, fileID string, limit int64) ([]byte, error)
}

// Gateway is the part of the gateway-api client the bot uses.
type Gateway interface {
	ScanText(ctx context.Context, text, lang, senderType string) (gateway.ScanResponse, error)
	ScanQR(ctx context.Context, image []byte, caption, lang string) (gateway.ScanResponse, error)
}

// Limits matching gateway-api.
const (
	MaxTextChars  = 5000
	MaxImageBytes = 5 << 20
)

// Bot handles updates.
type Bot struct {
	API     API
	Gateway Gateway
	Log     *slog.Logger
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
	if cmd, _ := command(m.Text); cmd != "" {
		switch cmd {
		case "/start", "/help":
			b.send(ctx, m.Chat.ID, welcome, nil)
		default:
			b.send(ctx, m.Chat.ID, "Sorry, I don't know that command. Send /help to see what I can do.", nil)
		}
		return
	}
	if fileID, ok := imageFileID(m); ok {
		b.handlePhoto(ctx, m, fileID)
		return
	}
	if m.Document != nil {
		b.send(ctx, m.Chat.ID, "Please send the QR code as a photo or an image file (PNG or JPEG).", nil)
		return
	}
	if strings.TrimSpace(m.Text) != "" {
		b.handleText(ctx, m)
	}
}

// imageFileID picks the best image in a message: the largest photo size
// under the limit, or an image sent as a document.
func imageFileID(m *telegram.Message) (string, bool) {
	best := ""
	bestArea := 0
	for _, p := range m.Photo {
		if p.FileSize > MaxImageBytes {
			continue
		}
		if a := p.Width * p.Height; a > bestArea {
			best, bestArea = p.FileID, a
		}
	}
	if best != "" {
		return best, true
	}
	if d := m.Document; d != nil && strings.HasPrefix(d.MimeType, "image/") && d.FileSize <= MaxImageBytes {
		return d.FileID, true
	}
	return "", false
}

func (b *Bot) handlePhoto(ctx context.Context, m *telegram.Message, fileID string) {
	img, err := b.API.DownloadFile(ctx, fileID, MaxImageBytes)
	if err != nil {
		if errors.Is(err, telegram.ErrFileTooLarge) {
			b.send(ctx, m.Chat.ID, "That image is too large. Please send a screenshot under 5 MB.", nil)
			return
		}
		b.scanFailed(ctx, m.Chat.ID, "qr", err)
		return
	}
	caption := strings.TrimSpace(m.Caption)
	resp, err := b.Gateway.ScanQR(ctx, img, caption, "en")
	if err != nil {
		b.scanFailed(ctx, m.Chat.ID, "qr", err)
		return
	}
	scansTotal.WithLabelValues("qr", resp.Verdict).Inc()
	if len(resp.QR) == 0 {
		b.send(ctx, m.Chat.ID, "I couldn't find a QR code in that image. Try a sharper, closer screenshot.", nil)
		return
	}
	b.send(ctx, m.Chat.ID, plainReply(resp), nil)
}

func (b *Bot) handleText(ctx context.Context, m *telegram.Message) {
	text := strings.TrimSpace(m.Text)
	if utf8.RuneCountInString(text) > MaxTextChars {
		text = string([]rune(text)[:MaxTextChars])
	}
	sender := "unknown"
	resp, err := b.Gateway.ScanText(ctx, text, "en", sender)
	if err != nil {
		b.scanFailed(ctx, m.Chat.ID, "text", err)
		return
	}
	scansTotal.WithLabelValues("text", resp.Verdict).Inc()
	b.send(ctx, m.Chat.ID, plainReply(resp), nil)
}

func (b *Bot) scanFailed(ctx context.Context, chatID int64, kind string, err error) {
	outcome := "error"
	msg := "Sorry, I couldn't check that right now. Please try again in a minute.\n\n" +
		"Meanwhile: never share your OTP, UPI PIN or password."
	if errors.Is(err, gateway.ErrBadInput) {
		outcome = "bad_input"
		msg = "Sorry, I couldn't read that. Please send the message text, or a clear screenshot of the QR code."
	}
	scansTotal.WithLabelValues(kind, outcome).Inc()
	b.Log.Warn("scan failed", "kind", kind, "err", err)
	b.send(ctx, chatID, msg, nil)
}

// plainReply is a minimal verdict rendering.
func plainReply(r gateway.ScanResponse) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (risk %d/100)\n", r.Verdict, r.RiskScore)
	for _, q := range r.QR {
		if q.UPI != nil && q.UPI.Summary != "" {
			fmt.Fprintf(&sb, "%s\n", q.UPI.Summary)
		}
	}
	for _, reason := range r.Reasons {
		fmt.Fprintf(&sb, "- %s\n", reason.Message)
	}
	return sb.String()
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
