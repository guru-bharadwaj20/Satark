package sandboxupdate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func phishResult() Result {
	return Result{
		ID: "ls-1", URL: "https://bit.example/x", Phase: "Succeeded",
		FinalURL:      "https://kyc-update.example/login?u=1",
		RedirectChain: []string{"https://bit.example/x", "https://t.example/", "https://kyc-update.example/login?u=1"},
		Forms:         Forms{OTP: true, CardNumber: true},
		Downloads:     []string{"PM-Kisan.apk"},
		Correlation:   map[string]string{KeyChatID: "1001", KeyMessageID: "55", KeyLang: "en"},
	}
}

func TestFormatSection(t *testing.T) {
	s := FormatSection(phishResult())
	for _, want := range []string{"Redirects: 2", "kyc-update.example", "OTP / PIN, card number", "PM-Kisan.apk"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "?u=1") {
		t.Error("full URL must not be echoed")
	}
}

func TestFormatNeverClaimsSafe(t *testing.T) {
	for _, lang := range []string{"en", "hi", "kn", "xx"} {
		r := Result{Phase: "Succeeded", FinalURL: "https://example.com/", Correlation: map[string]string{KeyLang: lang}}
		s := strings.ToLower(FormatSection(r))
		if strings.Contains(s, "100%") || strings.Contains(s, "is safe") {
			t.Errorf("%s: overclaims safety: %s", lang, s)
		}
	}
}

func TestFormatFailureAndTimeoutLocalised(t *testing.T) {
	r := Result{Phase: "TimedOut", Correlation: map[string]string{KeyLang: "hi"}}
	if !strings.Contains(FormatSection(r), "सैंडबॉक्स") {
		t.Error("expected Hindi text")
	}
	r = Result{Phase: "Failed", Correlation: map[string]string{KeyLang: "kn"}}
	if !strings.Contains(FormatSection(r), "ಸ್ಯಾಂಡ್‌ಬಾಕ್ಸ್") {
		t.Error("expected Kannada text")
	}
}

func TestFormatEscapesHTML(t *testing.T) {
	r := Result{Phase: "Succeeded", Downloads: []string{"<b>x</b>.apk"}}
	if strings.Contains(FormatSection(r), "<b>x</b>") {
		t.Error("download name not escaped")
	}
}

func TestCompose(t *testing.T) {
	out := Compose("⚠️ Suspicious link\n", phishResult())
	if !strings.HasPrefix(out, "⚠️ Suspicious link\n\n🔎") {
		t.Fatalf("unexpected compose: %q", out)
	}
}

func telegramServer(t *testing.T, status int, resp string, got *map[string]any) *BotAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/botTOKEN/editMessageText" {
			t.Errorf("path %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if got != nil {
			_ = json.Unmarshal(b, got)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return &BotAPI{Token: "TOKEN", BaseURL: srv.URL}
}

func TestBotAPI(t *testing.T) {
	ctx := context.Background()
	var got map[string]any
	api := telegramServer(t, 200, `{"ok":true}`, &got)
	if err := api.EditMessageText(ctx, 1001, 55, "hi"); err != nil {
		t.Fatal(err)
	}
	if got["parse_mode"] != "HTML" || got["message_id"].(float64) != 55 {
		t.Fatalf("request %v", got)
	}

	api = telegramServer(t, 429, `{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":7}}`, nil)
	var ra *RetryAfterError
	if err := api.EditMessageText(ctx, 1, 2, "x"); !errors.As(err, &ra) || ra.After != 7*time.Second {
		t.Fatalf("want retry-after, got %v", err)
	}

	api = telegramServer(t, 400, `{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`, nil)
	if err := api.EditMessageText(ctx, 1, 2, "x"); err != nil {
		t.Fatalf("not-modified should succeed, got %v", err)
	}

	api = telegramServer(t, 400, `{"ok":false,"error_code":400,"description":"Bad Request: message to edit not found"}`, nil)
	if err := api.EditMessageText(ctx, 1, 2, "x"); !errors.Is(err, ErrPermanent) {
		t.Fatalf("want permanent, got %v", err)
	}
}

type fakeEditor struct {
	calls []string
	err   error
}

func (f *fakeEditor) EditMessageText(_ context.Context, chatID, messageID int64, text string) error {
	f.calls = append(f.calls, text)
	return f.err
}

type fakeMsg struct {
	data                 []byte
	acked, termed, naked bool
}

func (m *fakeMsg) Data() []byte                     { return m.data }
func (m *fakeMsg) Ack() error                       { m.acked = true; return nil }
func (m *fakeMsg) Term() error                      { m.termed = true; return nil }
func (m *fakeMsg) NakWithDelay(time.Duration) error { m.naked = true; return nil }

type staticStore string

func (s staticStore) Original(context.Context, int64, int64) (string, bool) { return string(s), true }

func TestHandleOutcomes(t *testing.T) {
	ctx := context.Background()
	body, _ := json.Marshal(phishResult())

	ed := &fakeEditor{}
	u := &Updater{Editor: ed, Original: staticStore("Fast verdict")}
	m := &fakeMsg{data: body}
	u.Handle(ctx, m)
	if !m.acked || len(ed.calls) != 1 || !strings.HasPrefix(ed.calls[0], "Fast verdict") {
		t.Fatalf("ack=%v calls=%v", m.acked, ed.calls)
	}

	// No Telegram correlation: ack without editing.
	ed = &fakeEditor{}
	u = &Updater{Editor: ed}
	m = &fakeMsg{data: []byte(`{"id":"x","phase":"Succeeded"}`)}
	u.Handle(ctx, m)
	if !m.acked || len(ed.calls) != 0 {
		t.Fatal("non-telegram result should be acked and skipped")
	}

	m = &fakeMsg{data: []byte(`not json`)}
	u.Handle(ctx, m)
	if !m.termed {
		t.Fatal("malformed should be terminated")
	}

	u = &Updater{Editor: &fakeEditor{err: &RetryAfterError{After: time.Second}}}
	m = &fakeMsg{data: body}
	u.Handle(ctx, m)
	if !m.naked {
		t.Fatal("rate-limited edit should be nak'd")
	}

	u = &Updater{Editor: &fakeEditor{err: ErrPermanent}}
	m = &fakeMsg{data: body}
	u.Handle(ctx, m)
	if !m.termed {
		t.Fatal("permanent failure should be terminated")
	}
}

func TestRunConsumesFromJetStream(t *testing.T) {
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	s := natstest.RunServer(&opts)
	defer s.Shutdown()
	nc, err := nats.Connect(s.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, _ := jetstream.New(nc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: ResultsStream, Subjects: []string{"scan.results.>"}}); err != nil {
		t.Fatal(err)
	}
	cons, err := EnsureConsumer(ctx, js)
	if err != nil {
		t.Fatal(err)
	}
	ed := &syncEditor{done: make(chan string, 1)}
	go func() { _ = (&Updater{Editor: ed}).Run(ctx, cons) }()

	body, _ := json.Marshal(phishResult())
	if _, err := js.Publish(ctx, "scan.results.ls-1", body); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-ed.done:
		if !strings.Contains(text, "Sandbox check") {
			t.Fatalf("unexpected text %q", text)
		}
	case <-ctx.Done():
		t.Fatal("result not consumed")
	}
}

type syncEditor struct{ done chan string }

func (s *syncEditor) EditMessageText(_ context.Context, _, _ int64, text string) error {
	s.done <- text
	return nil
}
