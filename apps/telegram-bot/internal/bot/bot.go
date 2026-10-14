package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/gateway"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/i18n"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/reply"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

var (
	scansTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_telegram_scans_total",
		Help: "Scans requested through the bot, by kind and outcome.",
	}, []string{"kind", "outcome"})
	feedbackTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_telegram_feedback_total",
		Help: "Feedback button presses, by vote.",
	}, []string{"vote"})
)

// API is the part of the Telegram client the bot uses.
type API interface {
	SendMessage(ctx context.Context, m telegram.SendMessage) (telegram.Message, error)
	DownloadFile(ctx context.Context, fileID string, limit int64) ([]byte, error)
	AnswerCallbackQuery(ctx context.Context, id, text string) error
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

// Callback data prefixes. Telegram limits callback data to 64 bytes.
const (
	cbLang = "lang:"
	cbWho  = "who:" // who:<token>:<sender_type>
)

// senderOptions are the "Who sent this?" answers, mapped to gateway-api
// sender_type values.
var senderOptions = []struct {
	key    i18n.Key
	sender string
}{
	{i18n.SenderUnknown, "unknown_number"},
	{i18n.SenderSaved, "saved_contact"},
	{i18n.SenderBusiness, "business_sender"},
	{i18n.SenderGroup, "group"},
	{i18n.SenderSkip, "unknown"},
}

func validSender(s string) bool {
	for _, o := range senderOptions {
		if o.sender == s {
			return true
		}
	}
	return false
}

// Bot handles updates.
type Bot struct {
	API     API
	Gateway Gateway
	Prefs   *Prefs
	Pending *Pending
	Log     *slog.Logger
}

// Handle implements Handler.
func (b *Bot) Handle(ctx context.Context, u telegram.Update) {
	if b.Prefs == nil {
		b.Prefs = NewPrefs()
	}
	if b.Pending == nil {
		b.Pending = NewPending(10*time.Minute, 10000)
	}
	switch {
	case u.Message != nil:
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

func (b *Bot) langFor(chatID int64, from *telegram.User) string {
	code := ""
	if from != nil {
		code = from.LanguageCode
	}
	return b.Prefs.Lang(chatID, code)
}

func (b *Bot) handleMessage(ctx context.Context, m *telegram.Message) {
	if m.Chat.Type != "private" {
		return // the bot only talks in private chats
	}
	lang := b.langFor(m.Chat.ID, m.From)
	if cmd, arg := command(m.Text); cmd != "" {
		switch cmd {
		case "/start", "/help":
			b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.Welcome), nil)
		case "/language", "/lang", "/bhasha":
			if b.Prefs.SetLang(m.Chat.ID, strings.ToLower(arg)) {
				b.send(ctx, m.Chat.ID, i18n.T(strings.ToLower(arg), i18n.LanguageSet), nil)
				return
			}
			b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.ChooseLanguage), languageKeyboard())
		default:
			b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.UnknownCommand), nil)
		}
		return
	}
	if fileID, ok := imageFileID(m); ok {
		b.handlePhoto(ctx, m, fileID, lang)
		return
	}
	if m.Document != nil {
		b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.SendAsPhoto), nil)
		return
	}
	if strings.TrimSpace(m.Text) != "" {
		b.handleText(ctx, m, lang)
	}
}

func languageKeyboard() *telegram.InlineKeyboardMarkup {
	row := make([]telegram.InlineKeyboardButton, 0, len(i18n.Langs))
	for _, l := range i18n.Langs {
		row = append(row, telegram.InlineKeyboardButton{Text: i18n.LangNames[l], CallbackData: cbLang + l})
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{row}}
}

func (b *Bot) handleCallback(ctx context.Context, q *telegram.CallbackQuery) {
	if q.Message == nil || q.Message.Chat.Type != "private" {
		b.answer(ctx, q.ID, "")
		return
	}
	chatID := q.Message.Chat.ID
	switch {
	case strings.HasPrefix(q.Data, cbLang):
		l := strings.TrimPrefix(q.Data, cbLang)
		if !b.Prefs.SetLang(chatID, l) {
			b.answer(ctx, q.ID, "")
			return
		}
		b.answer(ctx, q.ID, i18n.T(l, i18n.LanguageSet))
		b.send(ctx, chatID, i18n.T(l, i18n.LanguageSet), nil)
	case strings.HasPrefix(q.Data, reply.FeedbackPrefix):
		scanID, helpful, ok := reply.ParseFeedback(q.Data)
		if !ok {
			b.answer(ctx, q.ID, "")
			return
		}
		vote := "down"
		if helpful {
			vote = "up"
		}
		feedbackTotal.WithLabelValues(vote).Inc()
		b.Log.Info("feedback", "scan_id", scanID, "vote", vote)
		b.answer(ctx, q.ID, reply.Label(b.langFor(chatID, &q.From), "thanks"))
	case strings.HasPrefix(q.Data, cbWho):
		token, sender, ok := strings.Cut(strings.TrimPrefix(q.Data, cbWho), ":")
		lang := b.langFor(chatID, &q.From)
		if !ok || !validSender(sender) {
			b.answer(ctx, q.ID, "")
			return
		}
		text, found := b.Pending.Take(chatID, token)
		b.answer(ctx, q.ID, "")
		if !found {
			b.send(ctx, chatID, i18n.T(lang, i18n.Expired), nil)
			return
		}
		b.scanText(ctx, chatID, text, lang, sender)
	default:
		b.answer(ctx, q.ID, "")
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

func (b *Bot) handlePhoto(ctx context.Context, m *telegram.Message, fileID, lang string) {
	img, err := b.API.DownloadFile(ctx, fileID, MaxImageBytes)
	if err != nil {
		if errors.Is(err, telegram.ErrFileTooLarge) {
			b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.ImageTooLarge), nil)
			return
		}
		b.scanFailed(ctx, m.Chat.ID, "qr", lang, err)
		return
	}
	caption := strings.TrimSpace(m.Caption)
	resp, err := b.Gateway.ScanQR(ctx, img, caption, lang)
	if err != nil {
		b.scanFailed(ctx, m.Chat.ID, "qr", lang, err)
		return
	}
	scansTotal.WithLabelValues("qr", resp.Verdict).Inc()
	if len(resp.QR) == 0 {
		b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.NoQRFound), nil)
		return
	}
	b.send(ctx, m.Chat.ID, reply.Render(resp, lang), reply.Keyboard(resp, lang))
}

// handleText asks "Who sent this?" before scanning. The text waits in
// memory (not on disk) until the user answers or the entry expires.
func (b *Bot) handleText(ctx context.Context, m *telegram.Message, lang string) {
	text := strings.TrimSpace(m.Text)
	if utf8.RuneCountInString(text) > MaxTextChars {
		text = string([]rune(text)[:MaxTextChars])
	}
	token := b.Pending.Put(m.Chat.ID, text)
	b.send(ctx, m.Chat.ID, i18n.T(lang, i18n.WhoSent), senderKeyboard(token, lang))
}

func senderKeyboard(token, lang string) *telegram.InlineKeyboardMarkup {
	rows := make([][]telegram.InlineKeyboardButton, 0, len(senderOptions))
	for _, o := range senderOptions {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text: i18n.T(lang, o.key), CallbackData: cbWho + token + ":" + o.sender,
		}})
	}
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *Bot) scanText(ctx context.Context, chatID int64, text, lang, sender string) {
	resp, err := b.Gateway.ScanText(ctx, text, lang, sender)
	if err != nil {
		b.scanFailed(ctx, chatID, "text", lang, err)
		return
	}
	scansTotal.WithLabelValues("text", resp.Verdict).Inc()
	b.send(ctx, chatID, reply.Render(resp, lang), reply.Keyboard(resp, lang))
}

func (b *Bot) scanFailed(ctx context.Context, chatID int64, kind, lang string, err error) {
	outcome, key := "error", i18n.ScanFailed
	if errors.Is(err, gateway.ErrBadInput) {
		outcome, key = "bad_input", i18n.BadInput
	}
	scansTotal.WithLabelValues(kind, outcome).Inc()
	b.Log.Warn("scan failed", "kind", kind, "err", err)
	b.send(ctx, chatID, i18n.T(lang, key), nil)
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

func (b *Bot) answer(ctx context.Context, id, text string) {
	if err := b.API.AnswerCallbackQuery(ctx, id, text); err != nil {
		b.Log.Debug("answerCallbackQuery failed", "err", err)
	}
}
