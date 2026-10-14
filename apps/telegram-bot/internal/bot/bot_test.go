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
	mu   sync.Mutex
	sent []telegram.SendMessage
}

func (f *fakeAPI) SendMessage(_ context.Context, m telegram.SendMessage) (telegram.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return telegram.Message{MessageID: int64(len(f.sent))}, nil
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
	return &Bot{API: api, Gateway: gw, Log: discard}, api, gw
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
