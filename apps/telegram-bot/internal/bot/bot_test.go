package bot

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/gateway"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

type fakeAPI struct {
	mu         sync.Mutex
	sent       []telegram.SendMessage
	downloaded []string
	dlErr      error
	answered   []string
}

func (f *fakeAPI) SendMessage(_ context.Context, m telegram.SendMessage) (telegram.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return telegram.Message{MessageID: int64(len(f.sent))}, nil
}

func (f *fakeAPI) DownloadFile(_ context.Context, fileID string, _ int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downloaded = append(f.downloaded, fileID)
	if f.dlErr != nil {
		return nil, f.dlErr
	}
	return []byte("img:" + fileID), nil
}

func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, id, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, id)
	return nil
}

func (f *fakeAPI) last() telegram.SendMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[len(f.sent)-1]
}

type fakeGateway struct {
	text, lang, sender string
	image              []byte
	caption            string
	err                error
	resp               gateway.ScanResponse
}

func (g *fakeGateway) ScanText(_ context.Context, text, lang, sender string) (gateway.ScanResponse, error) {
	g.text, g.lang, g.sender = text, lang, sender
	r := g.resp
	if r.Verdict == "" {
		r = gateway.ScanResponse{ScanID: "s1", Verdict: "likely_scam", RiskScore: 91, Lang: lang,
			Reasons: []gateway.Reason{{Code: "urgency.deadline", Message: "It gives a tight deadline"}}}
	}
	return r, g.err
}

func (g *fakeGateway) ScanQR(_ context.Context, image []byte, caption, lang string) (gateway.ScanResponse, error) {
	g.image, g.caption, g.lang = image, caption, lang
	r := g.resp
	if r.Verdict == "" {
		r = gateway.ScanResponse{ScanID: "q1", Verdict: "likely_scam", RiskScore: 80, Lang: lang}
	}
	return r, g.err
}

func privateText(text string) telegram.Update {
	return telegram.Update{UpdateID: 1, Message: &telegram.Message{
		MessageID: 10, Chat: telegram.Chat{ID: 42, Type: "private"}, Text: text,
		From: &telegram.User{ID: 7}, ForwardOrigin: &telegram.MessageOrigin{Type: "hidden_user"},
	}}
}

func newBot() (*Bot, *fakeAPI, *fakeGateway) {
	api, gw := &fakeAPI{}, &fakeGateway{}
	return &Bot{API: api, Gateway: gw, Prefs: NewPrefs(), Log: discard}, api, gw
}

func TestForwardedTextIsScanned(t *testing.T) {
	b, api, gw := newBot()
	b.Handle(context.Background(), privateText("  Your electricity will be disconnected tonight  "))
	if gw.text != "Your electricity will be disconnected tonight" {
		t.Fatalf("gateway got %q", gw.text)
	}
	if !strings.Contains(api.last().Text, "tight deadline") {
		t.Fatalf("reply %q", api.last().Text)
	}
}

func TestGroupMessagesIgnored(t *testing.T) {
	b, api, gw := newBot()
	u := privateText("hello")
	u.Message.Chat.Type = "group"
	b.Handle(context.Background(), u)
	if gw.text != "" || len(api.sent) != 0 {
		t.Fatal("group message should be ignored")
	}
}

func TestGatewayErrorReply(t *testing.T) {
	b, api, gw := newBot()
	gw.err = gateway.ErrUnavailable
	b.Handle(context.Background(), privateText("hello"))
	if !strings.Contains(api.last().Text, "try again") {
		t.Fatalf("reply %q", api.last().Text)
	}
}

func TestStartCommand(t *testing.T) {
	b, api, gw := newBot()
	b.Handle(context.Background(), privateText("/start"))
	if gw.text != "" || !strings.Contains(api.last().Text, "Satark") {
		t.Fatal("expected welcome without scan")
	}
}

func TestLongTextTruncated(t *testing.T) {
	b, _, gw := newBot()
	b.Handle(context.Background(), privateText(strings.Repeat("क", MaxTextChars+10)))
	if n := len([]rune(gw.text)); n != MaxTextChars {
		t.Fatalf("sent %d runes", n)
	}
}

func photoUpdate(caption string) telegram.Update {
	return telegram.Update{UpdateID: 2, Message: &telegram.Message{
		MessageID: 11, Chat: telegram.Chat{ID: 42, Type: "private"}, Caption: caption,
		Photo: []telegram.PhotoSize{
			{FileID: "small", Width: 90, Height: 90, FileSize: 1000},
			{FileID: "large", Width: 1280, Height: 1280, FileSize: 200000},
			{FileID: "huge", Width: 4000, Height: 4000, FileSize: MaxImageBytes + 1},
		},
	}}
}

func TestPhotoPicksLargestUnderLimit(t *testing.T) {
	b, api, gw := newBot()
	gw.resp = gateway.ScanResponse{Verdict: "likely_scam", RiskScore: 80,
		QR: []gateway.QRResult{{Kind: "upi", UPI: &gateway.UPIIntent{Summary: "This SENDS ₹14,999 FROM your account"}}}}
	b.Handle(context.Background(), photoUpdate(" scan to receive refund "))
	if len(api.downloaded) != 1 || api.downloaded[0] != "large" {
		t.Fatalf("downloaded %v", api.downloaded)
	}
	if string(gw.image) != "img:large" || gw.caption != "scan to receive refund" {
		t.Fatalf("gateway got %q %q", gw.image, gw.caption)
	}
	if !strings.Contains(api.last().Text, "SENDS") {
		t.Fatalf("reply %q", api.last().Text)
	}
}

func TestPhotoWithoutQR(t *testing.T) {
	b, api, gw := newBot()
	gw.resp = gateway.ScanResponse{Verdict: "no_red_flags"}
	b.Handle(context.Background(), photoUpdate(""))
	if !strings.Contains(api.last().Text, "couldn't find a QR") {
		t.Fatalf("reply %q", api.last().Text)
	}
}

func TestImageDocumentAccepted(t *testing.T) {
	b, api, _ := newBot()
	u := privateText("")
	u.Message.Document = &telegram.Document{FileID: "doc", MimeType: "image/png", FileSize: 10}
	b.Handle(context.Background(), u)
	if len(api.downloaded) != 1 || api.downloaded[0] != "doc" {
		t.Fatalf("downloaded %v", api.downloaded)
	}
}

func TestPhotoTooLarge(t *testing.T) {
	b, api, gw := newBot()
	api.dlErr = telegram.ErrFileTooLarge
	b.Handle(context.Background(), photoUpdate(""))
	if gw.image != nil || !strings.Contains(api.last().Text, "too large") {
		t.Fatalf("reply %q", api.last().Text)
	}
}

func callback(data string) telegram.Update {
	return telegram.Update{UpdateID: 3, CallbackQuery: &telegram.CallbackQuery{
		ID: "cb1", From: telegram.User{ID: 7}, Data: data,
		Message: &telegram.Message{MessageID: 12, Chat: telegram.Chat{ID: 42, Type: "private"}},
	}}
}

func TestLanguageDefaultsFromTelegramClient(t *testing.T) {
	b, _, gw := newBot()
	u := privateText("hello")
	u.Message.From.LanguageCode = "kn-IN"
	b.Handle(context.Background(), u)
	if gw.lang != "kn" {
		t.Fatalf("lang %q", gw.lang)
	}
}

func TestLanguageMenuAndCallback(t *testing.T) {
	b, api, gw := newBot()
	b.Handle(context.Background(), privateText("/language"))
	kb := api.last().ReplyMarkup
	if kb == nil || len(kb.InlineKeyboard[0]) != 3 || kb.InlineKeyboard[0][1].CallbackData != "lang:hi" {
		t.Fatalf("keyboard %+v", kb)
	}
	b.Handle(context.Background(), callback("lang:hi"))
	if len(api.answered) != 1 || !strings.Contains(api.last().Text, "हिन्दी") {
		t.Fatalf("reply %q", api.last().Text)
	}
	b.Handle(context.Background(), privateText("hello"))
	if gw.lang != "hi" {
		t.Fatalf("lang %q", gw.lang)
	}
	b.Handle(context.Background(), privateText("/start"))
	if !strings.Contains(api.last().Text, "नमस्ते") {
		t.Fatalf("welcome not localized: %q", api.last().Text)
	}
}

func TestLanguageCommandArgument(t *testing.T) {
	b, _, gw := newBot()
	b.Handle(context.Background(), privateText("/lang KN"))
	b.Handle(context.Background(), privateText("hello"))
	if gw.lang != "kn" {
		t.Fatalf("lang %q", gw.lang)
	}
}

func TestBadLanguageCallbackIgnored(t *testing.T) {
	b, api, _ := newBot()
	b.Handle(context.Background(), callback("lang:fr"))
	if len(api.answered) != 1 || len(api.sent) != 0 {
		t.Fatal("unsupported language should only be acknowledged")
	}
}
