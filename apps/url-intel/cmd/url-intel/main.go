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

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/config"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/dnscheck"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/domainage"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/expand"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/netguard"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/normalize"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/server"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/tlscheck"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.FromEnv()

	lookupClient := netguard.NewHTTPClient(cfg.LookupTimeout)

	an := analyzer.New(
		analyzer.WithPreparers(
			normalize.Preparer{},
			expand.Preparer{E: expand.New(cfg.ExpandTimeout, cfg.MaxRedirects)},
		),
		analyzer.WithChecks(
			dnscheck.New(),
			&domainage.Check{L: domainage.New(lookupClient)},
			tlscheck.New(),
		),
	)
	srv := server.New(an, nil, cfg.RequestTimeout, log)

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
