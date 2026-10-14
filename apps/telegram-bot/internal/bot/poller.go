// Package bot implements the Satark Telegram conversation.
package bot

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

var (
	updatesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "satark_telegram_updates_total",
		Help: "Telegram updates received, by type.",
	}, []string{"type"})
	pollErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "satark_telegram_poll_errors_total",
		Help: "Failed getUpdates calls.",
	})
	lastPoll = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "satark_telegram_last_poll_timestamp_seconds",
		Help: "Unix time of the last successful getUpdates call.",
	})
)

// Updater is the part of the Telegram client the poller needs.
type Updater interface {
	GetUpdates(ctx context.Context, offset int64, timeout time.Duration, allowed []string) ([]telegram.Update, error)
}

// Handler processes one update.
type Handler interface {
	Handle(ctx context.Context, u telegram.Update)
}

// Poller long-polls getUpdates and dispatches updates to a Handler.
type Poller struct {
	API         Updater
	Handler     Handler
	Log         *slog.Logger
	PollTimeout time.Duration // server-side long-poll duration
	Workers     int           // concurrent update handlers
	HandleTTL   time.Duration // deadline for handling one update

	mu      sync.Mutex
	healthy time.Time
}

// AllowedUpdates are the update types the bot subscribes to.
var AllowedUpdates = []string{"message", "callback_query"}

// Healthy reports whether a poll succeeded within the given window.
func (p *Poller) Healthy(window time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.healthy.IsZero() && time.Since(p.healthy) < window
}

// Run polls until ctx is cancelled, then waits for in-flight handlers.
func (p *Poller) Run(ctx context.Context) error {
	workers := max(p.Workers, 1)
	jobs := make(chan telegram.Update)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				p.dispatch(u)
			}
		}()
	}
	defer func() {
		close(jobs)
		wg.Wait()
	}()

	var offset int64
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return nil
		}
		ups, err := p.API.GetUpdates(ctx, offset, p.PollTimeout, AllowedUpdates)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			pollErrors.Inc()
			wait := backoff
			var apiErr *telegram.APIError
			if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
				wait = time.Duration(apiErr.RetryAfter) * time.Second
			}
			wait += time.Duration(rand.Int64N(int64(wait / 4)))
			p.Log.Warn("getUpdates failed", "err", err, "retry_in", wait.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(wait):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
		p.mu.Lock()
		p.healthy = time.Now()
		p.mu.Unlock()
		lastPoll.SetToCurrentTime()
		for _, u := range ups {
			offset = max(offset, u.UpdateID+1)
			select {
			case jobs <- u:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (p *Poller) dispatch(u telegram.Update) {
	switch {
	case u.Message != nil:
		updatesTotal.WithLabelValues("message").Inc()
	case u.CallbackQuery != nil:
		updatesTotal.WithLabelValues("callback_query").Inc()
	default:
		updatesTotal.WithLabelValues("other").Inc()
		return
	}
	ttl := p.HandleTTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	// Handlers get their own deadline so a shutdown does not cut off a
	// reply that is already being prepared.
	ctx, cancel := context.WithTimeout(context.Background(), ttl)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			p.Log.Error("handler panic", "panic", r, "update_id", u.UpdateID)
		}
	}()
	p.Handler.Handle(ctx, u)
}
