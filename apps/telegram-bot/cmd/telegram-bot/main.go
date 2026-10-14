// Command telegram-bot runs the Satark Telegram bot using long polling.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/bot"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/gateway"
	"github.com/guru-bharadwaj20/satark/apps/telegram-bot/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func run() error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(getenv("LOG_LEVEL", "info"))); err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).With("service", "telegram-bot")
	slog.SetDefault(log)

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is required")
	}
	pollTimeout, err := time.ParseDuration(getenv("POLL_TIMEOUT", "50s"))
	if err != nil {
		return fmt.Errorf("POLL_TIMEOUT: %w", err)
	}

	tg := telegram.New(getenv("TELEGRAM_API_URL", telegram.DefaultAPIURL), token, nil)
	gwTimeout, err := time.ParseDuration(getenv("GATEWAY_TIMEOUT", "10s"))
	if err != nil {
		return fmt.Errorf("GATEWAY_TIMEOUT: %w", err)
	}
	gw := gateway.New(getenv("GATEWAY_API_URL", "http://gateway-api:8080"), gwTimeout)
	b := &bot.Bot{API: tg, Gateway: gw, Prefs: bot.NewPrefs(), Pending: bot.NewPending(10*time.Minute, 10000), Log: log}
	poller := &bot.Poller{API: tg, Handler: b, Log: log, PollTimeout: pollTimeout, Workers: 8}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	me, err := tg.GetMe(startCtx)
	if err == nil {
		err = tg.DeleteWebhook(startCtx)
	}
	cancel()
	if err != nil {
		return fmt.Errorf("telegram startup: %w", err)
	}
	log.Info("bot authenticated", "username", me.Username)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !poller.Healthy(pollTimeout + 2*time.Minute) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"degraded"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("GET /metrics", promhttp.Handler())
	ops := &http.Server{Addr: getenv("ADDR", ":8082"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := ops.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("ops server", "err", err)
			stop()
		}
	}()

	log.Info("polling for updates")
	err = poller.Run(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = ops.Shutdown(shutdownCtx)
	return err
}
