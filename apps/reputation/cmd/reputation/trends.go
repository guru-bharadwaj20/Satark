package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/trends"
)

// trendsCmd prints the anonymised monthly scam-trends report as Markdown.
//
//	reputation trends [-month 2026-09]   (default: last full month)
func trendsCmd(_ *slog.Logger) error {
	fs := flag.NewFlagSet("trends", flag.ContinueOnError)
	monthFlag := fs.String("month", "", "month to report, YYYY-MM (default: previous month)")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	if *monthFlag != "" {
		m, err := time.Parse("2006-01", *monthFlag)
		if err != nil {
			return fmt.Errorf("invalid -month: %w", err)
		}
		start = m
	}
	end := start.AddDate(0, 1, 0)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	st, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()

	rows, err := st.Trends(ctx, start, end)
	if err != nil {
		return err
	}
	prev, err := st.Trends(ctx, start.AddDate(0, -1, 0), start)
	if err != nil {
		return err
	}
	fmt.Print(trends.Render(start, rows, prev))
	return nil
}
