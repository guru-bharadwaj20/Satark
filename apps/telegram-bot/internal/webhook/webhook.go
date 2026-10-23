// Package webhook receives Telegram updates over HTTPS webhooks instead of
// long polling.
//
// Telegram POSTs each update as JSON to the URL registered with setWebhook.
// The server here:
//
//   - accepts only POST on the configured path,
//   - checks the X-Telegram-Bot-Api-Secret-Token header in constant time,
//   - limits the request body size,
//   - drops duplicate update_ids (Telegram retries on non-2xx responses),
//   - queues updates to a bounded worker pool and answers 200 at once, so a
//     slow scan never holds the webhook connection open. When the queue is
//     full it answers 503, and Telegram retries later.
//
// The package only depends on the standard library and hands updates to the
// bot as raw JSON, so it can sit in front of any update-handling code.
package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// SecretHeader is the header Telegram sets to the secret_token given to setWebhook.
const SecretHeader = "X-Telegram-Bot-Api-Secret-Token"

// DefaultMaxBodyBytes is large enough for any Telegram update (they are small;
// files are referenced by file_id, not inlined).
const DefaultMaxBodyBytes = 1 << 20

// Update is one Telegram update. Only update_id is decoded here, and the raw
// JSON is passed on unchanged.
type Update struct {
	ID  int64
	Raw json.RawMessage
}

// HandlerFunc processes one update. A returned error is logged. It never
// makes Telegram redeliver, because the update has already been acknowledged.
type HandlerFunc func(ctx context.Context, u Update) error

// Config configures a Server.
type Config struct {
	// Secret must match the secret_token passed to setWebhook. Required.
	Secret string
	// Path the webhook is served on, e.g. "/telegram/webhook". Defaults to "/".
	Path string
	// Workers is the number of goroutines processing updates. Default 4.
	Workers int
	// QueueSize bounds the number of accepted but unprocessed updates. Default 100.
	QueueSize int
	// HandlerTimeout bounds each handler call. Default 60s.
	HandlerTimeout time.Duration
	// MaxBodyBytes limits the request body. Default DefaultMaxBodyBytes.
	MaxBodyBytes int64
	// DedupSize is how many recent update_ids are remembered. Default 1024.
	DedupSize int
	// Logger defaults to slog.Default().
	Logger *slog.Logger
}

// Stats are counters for metrics and tests.
type Stats struct {
	Accepted     uint64
	Duplicates   uint64
	Unauthorized uint64
	BadRequests  uint64
	QueueFull    uint64
	Processed    uint64
	Failed       uint64
}

// Server is an http.Handler for Telegram webhooks.
type Server struct {
	cfg     Config
	handler HandlerFunc
	log     *slog.Logger

	queue   chan Update
	dedup   *recentIDs
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	closed  atomic.Bool
	closeMu sync.RWMutex

	accepted, duplicates, unauthorized, badRequests, queueFull, processed, failed atomic.Uint64
}

// ErrNoSecret is returned by New when Config.Secret is empty.
var ErrNoSecret = errors.New("webhook: secret token is required")

// New creates a Server and starts its workers. Call Shutdown to stop them.
func New(cfg Config, h HandlerFunc) (*Server, error) {
	if cfg.Secret == "" {
		return nil, ErrNoSecret
	}
	if !validSecret(cfg.Secret) {
		return nil, errors.New("webhook: secret must be 1-256 characters of A-Z, a-z, 0-9, _ or -")
	}
	if h == nil {
		return nil, errors.New("webhook: handler is required")
	}
	if cfg.Path == "" {
		cfg.Path = "/"
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 100
	}
	if cfg.HandlerTimeout <= 0 {
		cfg.HandlerTimeout = 60 * time.Second
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if cfg.DedupSize <= 0 {
		cfg.DedupSize = 1024
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		cfg:     cfg,
		handler: h,
		log:     cfg.Logger.With("component", "telegram-webhook"),
		queue:   make(chan Update, cfg.QueueSize),
		dedup:   newRecentIDs(cfg.DedupSize),
		ctx:     ctx,
		cancel:  cancel,
	}
	for i := 0; i < cfg.Workers; i++ {
		s.wg.Add(1)
		go s.worker()
	}
	return s, nil
}

// validSecret enforces Telegram's allowed secret_token alphabet.
func validSecret(s string) bool {
	if len(s) < 1 || len(s) > 256 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != s.cfg.Path {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	got := r.Header.Get(SecretHeader)
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Secret)) != 1 {
		s.unauthorized.Add(1)
		// Don't log the received value: it may be someone's guess at a secret.
		s.log.Warn("rejected webhook request with bad secret", "remote", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes))
	if err != nil {
		s.badRequests.Add(1)
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var head struct {
		UpdateID *int64 `json:"update_id"`
	}
	if err := json.Unmarshal(body, &head); err != nil || head.UpdateID == nil {
		s.badRequests.Add(1)
		http.Error(w, "invalid update", http.StatusBadRequest)
		return
	}
	u := Update{ID: *head.UpdateID, Raw: json.RawMessage(body)}

	if !s.dedup.add(u.ID) {
		s.duplicates.Add(1)
		w.WriteHeader(http.StatusOK)
		return
	}

	if !s.enqueue(u) {
		// Forget the ID so Telegram's retry is accepted.
		s.dedup.remove(u.ID)
		s.queueFull.Add(1)
		w.Header().Set("Retry-After", "5")
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	s.accepted.Add(1)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) enqueue(u Update) bool {
	s.closeMu.RLock()
	defer s.closeMu.RUnlock()
	if s.closed.Load() {
		return false
	}
	select {
	case s.queue <- u:
		return true
	default:
		return false
	}
}

func (s *Server) worker() {
	defer s.wg.Done()
	for u := range s.queue {
		s.process(u)
	}
}

func (s *Server) process(u Update) {
	ctx, cancel := context.WithTimeout(s.ctx, s.cfg.HandlerTimeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			s.failed.Add(1)
			s.log.Error("update handler panicked", "update_id", u.ID, "panic", p)
		}
	}()
	if err := s.handler(ctx, u); err != nil {
		s.failed.Add(1)
		s.log.Error("update handler failed", "update_id", u.ID, "err", err)
		return
	}
	s.processed.Add(1)
}

// Shutdown stops accepting updates, lets the workers drain the queue, and
// returns when they finish or ctx expires. When ctx expires, running handlers
// see their contexts cancelled.
func (s *Server) Shutdown(ctx context.Context) error {
	s.closeMu.Lock()
	if !s.closed.Swap(true) {
		close(s.queue)
	}
	s.closeMu.Unlock()

	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		s.cancel()
		return nil
	case <-ctx.Done():
		s.cancel()
		return ctx.Err()
	}
}

// Stats returns a snapshot of the counters.
func (s *Server) Stats() Stats {
	return Stats{
		Accepted:     s.accepted.Load(),
		Duplicates:   s.duplicates.Load(),
		Unauthorized: s.unauthorized.Load(),
		BadRequests:  s.badRequests.Load(),
		QueueFull:    s.queueFull.Load(),
		Processed:    s.processed.Load(),
		Failed:       s.failed.Load(),
	}
}

// QueueDepth returns the number of queued, unprocessed updates.
func (s *Server) QueueDepth() int { return len(s.queue) }

// recentIDs is a fixed-size FIFO set of recently seen update IDs.
type recentIDs struct {
	mu   sync.Mutex
	set  map[int64]struct{}
	ring []int64
	next int
	full bool
}

func newRecentIDs(n int) *recentIDs {
	return &recentIDs{set: make(map[int64]struct{}, n), ring: make([]int64, n)}
}

// add records id and reports whether it was new.
func (r *recentIDs) add(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.set[id]; ok {
		return false
	}
	if r.full {
		delete(r.set, r.ring[r.next])
	}
	r.ring[r.next] = id
	r.set[id] = struct{}{}
	r.next++
	if r.next == len(r.ring) {
		r.next = 0
		r.full = true
	}
	return true
}

// remove forgets id. Its ring slot stays in place. At worst, a later
// eviction of that slot forgets a re-added id early, which only weakens
// dedup for that one id.
func (r *recentIDs) remove(id int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.set, id)
}
