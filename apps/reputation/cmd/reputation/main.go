// Command reputation serves community reports for phone numbers, UPI IDs
// and domains.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/ratelimit"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cmd := run
	if len(os.Args) > 1 && os.Args[1] == "retention" {
		cmd = retention
	}
	if err := cmd(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return n
	}
	return def
}

func openStore(ctx context.Context) (store.Store, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return store.NewMemory(), nil
	}
	return store.NewPostgres(ctx, dsn)
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pepper := os.Getenv("REPUTATION_PEPPER")
	if pepper == "" {
		return errors.New("REPUTATION_PEPPER must be set")
	}
	st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer st.Close()

	srv := &http.Server{
		Addr: ":" + env("PORT", "8083"),
		Handler: api.New(api.Config{
			Store:        st,
			Hasher:       identifier.NewHasher(pepper),
			Logger:       log,
			MinReporters: envInt("REPUTATION_MIN_REPORTERS", 3),
			AdminToken:   os.Getenv("REPUTATION_ADMIN_TOKEN"),
			Limiter: ratelimit.New(nil,
				ratelimit.Window{Max: envInt("REPUTATION_REPORTS_PER_HOUR", 5), Period: time.Hour},
				ratelimit.Window{Max: envInt("REPUTATION_REPORTS_PER_DAY", 20), Period: 24 * time.Hour},
			),
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
