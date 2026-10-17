// Command gateway-api serves the public Satark REST API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/analyzers"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/config"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/hashing"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/ratelimit"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/scan"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/server"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/store"
)

// version is set at build time with -ldflags "-X main.version=..."
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	log = log.With("service", "gateway-api", "version", version)
	slog.SetDefault(log)

	engine := &scan.Service{
		Text:    analyzers.TextAnalyzer{Client: analyzers.NewClient("text-analyzer", cfg.TextAnalyzerURL, cfg.AnalyzerTimeout, nil)},
		QR:      analyzers.QRDecoder{Client: analyzers.NewClient("qr-decoder", cfg.QRDecoderURL, cfg.AnalyzerTimeout, nil)},
		Scorer:  scan.EngineScorer{},
		Timeout: cfg.ScanTimeout,
		Log:     log,
	}
	var scanner server.Scanner = engine
	srv := server.New(cfg, log)

	if cfg.DatabaseURL != "" {
		hasher, err := hashing.NewFromEnv()
		if err != nil {
			return err
		}
		pool, err := store.Connect(context.Background(), cfg.DatabaseURL)
		if err != nil {
			return err
		}
		defer pool.Close()
		scanner = &scan.Recorder{Next: engine, Store: store.New(pool, hasher, log), Log: log}
		log.Info("scan persistence enabled")
	}

	if cfg.ValkeyAddr != "" {
		rdb := redis.NewClient(&redis.Options{Addr: cfg.ValkeyAddr, Password: cfg.ValkeyPassword})
		defer rdb.Close()
		rl := ratelimit.New(rdb, ratelimit.Options{Logger: log})
		lim := ratelimit.Limit{Rate: cfg.RateLimitRPS, Burst: cfg.RateLimitBurst}
		srv.WithRateLimit(func(h http.Handler) http.Handler {
			return rl.Middleware(lim, ratelimit.ClientKey, h)
		})
		log.Info("rate limiting enabled", "rps", lim.Rate, "burst", lim.Burst)
	}

	srv.WithScanner(scanner)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		errCh <- httpSrv.ListenAndServe()
	}()
	srv.SetReady(true)

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		srv.SetReady(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}
