package webhook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testSecret = "s3cr3t_TOKEN-1"

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newServer(t *testing.T, cfg Config, h HandlerFunc) *Server {
	t.Helper()
	if cfg.Secret == "" {
		cfg.Secret = testSecret
	}
	cfg.Logger = quietLogger()
	s, err := New(cfg, h)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Shutdown(context.Background()) })
	return s
}

func post(s http.Handler, path, secret, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if secret != "" {
		req.Header.Set(SecretHeader, secret)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestNewValidation(t *testing.T) {
	h := func(context.Context, Update) error { return nil }
	if _, err := New(Config{}, h); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("want ErrNoSecret, got %v", err)
	}
	if _, err := New(Config{Secret: "has space"}, h); err == nil {
		t.Fatal("want error for invalid secret characters")
	}
	if _, err := New(Config{Secret: testSecret}, nil); err == nil {
		t.Fatal("want error for nil handler")
	}
}

func TestDeliversUpdate(t *testing.T) {
	got := make(chan Update, 1)
	s := newServer(t, Config{Path: "/tg"}, func(_ context.Context, u Update) error {
		got <- u
		return nil
	})
	rec := post(s, "/tg", testSecret, `{"update_id":42,"message":{"text":"hi"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	select {
	case u := <-got:
		if u.ID != 42 || !strings.Contains(string(u.Raw), `"hi"`) {
			t.Fatalf("unexpected update %+v", u)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called")
	}
}

func TestRejects(t *testing.T) {
	s := newServer(t, Config{Path: "/tg", MaxBodyBytes: 64}, func(context.Context, Update) error { return nil })
	cases := []struct {
		name, method, path, secret, body string
		want                             int
	}{
		{"wrong path", http.MethodPost, "/other", testSecret, `{"update_id":1}`, http.StatusNotFound},
		{"GET", http.MethodGet, "/tg", testSecret, "", http.StatusMethodNotAllowed},
		{"no secret", http.MethodPost, "/tg", "", `{"update_id":1}`, http.StatusUnauthorized},
		{"bad secret", http.MethodPost, "/tg", "nope", `{"update_id":1}`, http.StatusUnauthorized},
		{"bad json", http.MethodPost, "/tg", testSecret, `{`, http.StatusBadRequest},
		{"no update_id", http.MethodPost, "/tg", testSecret, `{"message":{}}`, http.StatusBadRequest},
		{"too large", http.MethodPost, "/tg", testSecret, `{"update_id":1,"x":"` + strings.Repeat("a", 100) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.secret != "" {
				req.Header.Set(SecretHeader, tc.secret)
			}
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d want %d", rec.Code, tc.want)
			}
		})
	}
	if st := s.Stats(); st.Unauthorized != 2 || st.Accepted != 0 {
		t.Fatalf("stats %+v", st)
	}
}

func TestDeduplicates(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s := newServer(t, Config{}, func(context.Context, Update) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})
	for i := 0; i < 3; i++ {
		if rec := post(s, "/", testSecret, `{"update_id":7}`); rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("handler called %d times", calls)
	}
	if st := s.Stats(); st.Duplicates != 2 || st.Processed != 1 {
		t.Fatalf("stats %+v", st)
	}
}

func TestQueueFullReturns503AndAllowsRetry(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 10)
	s := newServer(t, Config{Workers: 1, QueueSize: 1}, func(context.Context, Update) error {
		started <- struct{}{}
		<-release
		return nil
	})
	post(s, "/", testSecret, `{"update_id":1}`) // taken by the worker
	<-started
	post(s, "/", testSecret, `{"update_id":2}`) // fills the queue
	rec := post(s, "/", testSecret, `{"update_id":3}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
	close(release)
	// Wait for the queue to drain, then the retry of 3 must be accepted.
	deadline := time.Now().Add(2 * time.Second)
	for s.QueueDepth() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rec := post(s, "/", testSecret, `{"update_id":3}`); rec.Code != http.StatusOK {
		t.Fatalf("retry got %d", rec.Code)
	}
}

func TestHandlerErrorAndPanicCounted(t *testing.T) {
	s := newServer(t, Config{}, func(_ context.Context, u Update) error {
		if u.ID == 1 {
			return fmt.Errorf("boom")
		}
		panic("bad")
	})
	post(s, "/", testSecret, `{"update_id":1}`)
	post(s, "/", testSecret, `{"update_id":2}`)
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := s.Stats(); st.Failed != 2 {
		t.Fatalf("stats %+v", st)
	}
}

func TestShutdownRejectsNewUpdates(t *testing.T) {
	s := newServer(t, Config{}, func(context.Context, Update) error { return nil })
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rec := post(s, "/", testSecret, `{"update_id":9}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
	// A second Shutdown is a no-op.
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRecentIDsEvicts(t *testing.T) {
	r := newRecentIDs(2)
	if !r.add(1) || !r.add(2) || r.add(1) {
		t.Fatal("basic add failed")
	}
	r.add(3) // evicts 1
	if !r.add(1) {
		t.Fatal("1 should have been evicted")
	}
}
