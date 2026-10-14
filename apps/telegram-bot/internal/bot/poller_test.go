package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

type scriptedUpdater struct {
	mu      sync.Mutex
	calls   int
	offsets []int64
	batches [][]telegram.Update
	errs    []error
}

func (s *scriptedUpdater) GetUpdates(ctx context.Context, offset int64, _ time.Duration, _ []string) ([]telegram.Update, error) {
	s.mu.Lock()
	i := s.calls
	s.calls++
	s.offsets = append(s.offsets, offset)
	s.mu.Unlock()
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	if i < len(s.batches) {
		return s.batches[i], nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type recordingHandler struct {
	mu  sync.Mutex
	ids []int64
}

func (r *recordingHandler) Handle(_ context.Context, u telegram.Update) {
	r.mu.Lock()
	r.ids = append(r.ids, u.UpdateID)
	r.mu.Unlock()
}

func msg(id int64) telegram.Update {
	return telegram.Update{UpdateID: id, Message: &telegram.Message{Chat: telegram.Chat{Type: "private"}}}
}

func TestPollerAdvancesOffsetAndDispatches(t *testing.T) {
	up := &scriptedUpdater{batches: [][]telegram.Update{{msg(5), msg(6)}, {msg(7)}}}
	h := &recordingHandler{}
	p := &Poller{API: up, Handler: h, Log: discard, PollTimeout: time.Second, Workers: 2}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.ids) != 3 {
		t.Fatalf("handled %v", h.ids)
	}
	if up.offsets[0] != 0 || up.offsets[1] != 7 || up.offsets[2] != 8 {
		t.Fatalf("offsets %v", up.offsets)
	}
	if !p.Healthy(time.Minute) {
		t.Fatal("expected healthy")
	}
}

func TestPollerRetriesAfterError(t *testing.T) {
	up := &scriptedUpdater{
		errs:    []error{&telegram.APIError{Method: "getUpdates", Code: 429, RetryAfter: 0}, nil},
		batches: [][]telegram.Update{nil, {msg(1)}},
	}
	up.errs[0] = errors.New("network down")
	h := &recordingHandler{}
	p := &Poller{API: up, Handler: h, Log: discard, PollTimeout: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		for {
			h.mu.Lock()
			n := len(h.ids)
			h.mu.Unlock()
			if n > 0 {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	_ = p.Run(ctx)
	if len(h.ids) != 1 {
		t.Fatalf("handled %v after retry", h.ids)
	}
}

func TestCommandParsing(t *testing.T) {
	cases := map[string][2]string{
		"/start":              {"/start", ""},
		"/Language@SatarkBot": {"/language", ""},
		"/lang hi":            {"/lang", "hi"},
		"hello":               {"", ""},
	}
	for in, want := range cases {
		c, a := command(in)
		if c != want[0] || a != want[1] {
			t.Errorf("%q -> %q %q", in, c, a)
		}
	}
}
