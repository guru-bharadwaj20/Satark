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
	"syscall"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/identifier"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
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
		Addr:              ":" + env("PORT", "8083"),
		Handler:           api.New(api.Config{Store: st, Hasher: identifier.NewHasher(pepper), Logger: log}),
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
