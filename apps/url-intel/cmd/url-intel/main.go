// Command url-intel serves link-intelligence analysis for Satark.
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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/allowlist"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/cache"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/config"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/ctlog"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/dnscheck"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainage"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/expand"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/homoglyph"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/lookalike"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/normalize"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/safebrowsing"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/server"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/threatfeed"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/tlscheck"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/urlheur"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.FromEnv()

	lookupClient := netguard.NewHTTPClient(cfg.LookupTimeout)

	brands := lookalike.Default()

	sb := safebrowsing.New(lookupClient, cfg.SafeBrowsingAPIKey, cfg.Version)
	if !sb.Enabled() {
		log.Warn("Safe Browsing disabled: URL_INTEL_SAFE_BROWSING_API_KEY not set")
	}

	vk := redis.NewClient(&redis.Options{
		Addr: cfg.ValkeyAddr, Password: cfg.ValkeyPassword,
		DialTimeout: time.Second, ReadTimeout: 500 * time.Millisecond, WriteTimeout: 500 * time.Millisecond,
	})
	defer vk.Close()
	feedStore := &threatfeed.Store{C: vk, Feeds: threatfeed.DefaultFeeds}

	reg := prometheus.NewRegistry()
	checkDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "satark_url_intel_check_duration_seconds", Help: "Duration of individual checks by outcome.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	}, []string{"check", "outcome"})
	reg.MustRegister(checkDuration)

	an := analyzer.New(
		analyzer.WithWorkers(cfg.Workers),
		analyzer.WithCheckTimeout(cfg.CheckTimeout),
		analyzer.WithObserver(func(c string, d time.Duration, o string) {
			checkDuration.WithLabelValues(c, o).Observe(d.Seconds())
		}),
		analyzer.WithPreparers(
			normalize.Preparer{},
			expand.Preparer{E: expand.New(cfg.ExpandTimeout, cfg.MaxRedirects)},
		),
		analyzer.WithAllowlist(allowlist.Default(brands.OfficialDomains()...)),
		analyzer.WithChecks(
			dnscheck.New(),
			&domainage.Check{L: domainage.New(lookupClient)},
			tlscheck.New(),
			ctlog.New(lookupClient),
			&lookalike.Check{D: brands},
			&homoglyph.Check{Brands: brands},
			urlheur.Check{},
			&safebrowsing.Check{C: sb},
			&threatfeed.Check{S: feedStore},
		),
	)
	ready := func(ctx context.Context) error { return vk.Ping(ctx).Err() }
	cached := cache.New(an, vk, cfg.CacheTTL, cfg.CacheRiskyTTL, log, reg)
	srv := server.New(cached, ready, cfg.RequestTimeout, log)
	srv.Registry().MustRegister(threatfeed.NewCollector(feedStore), reg)

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("url-intel listening", "addr", cfg.ListenAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	srv.SetDraining()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown failed", "err", err)
	}
}
