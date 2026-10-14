package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/reputation/internal/store"
)

// retention runs one pass of the data retention policy and exits. It is
// run daily by the reputation-retention CronJob (deploy/retention-cronjob.yaml).
//
//	REPUTATION_RETENTION_DAYS          reports older than this are deleted (default 365)
//	REPUTATION_CONTACT_RETENTION_DAYS  requester contacts kept this long after a decision (default 30)
//	REPUTATION_DELISTING_RETENTION_DAYS decided delisting requests kept this long (default 365)
func retention(log *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL must be set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st, err := store.NewPostgres(ctx, dsn)
	if err != nil {
		return err
	}
	defer st.Close()

	now := time.Now().UTC()
	days := func(k string, def int) time.Time { return now.AddDate(0, 0, -envInt(k, def)) }
	pol := store.RetentionPolicy{
		ReportsBefore:        days("REPUTATION_RETENTION_DAYS", 365),
		ContactsBefore:       days("REPUTATION_CONTACT_RETENTION_DAYS", 30),
		DecidedDelistsBefore: days("REPUTATION_DELISTING_RETENTION_DAYS", 365),
	}
	res, err := st.Purge(ctx, pol)
	if err != nil {
		return err
	}
	log.Info("retention complete",
		"reports_deleted", res.Reports,
		"contacts_cleared", res.ContactsCleared,
		"delistings_deleted", res.Delistings,
		"reports_before", pol.ReportsBefore.Format(time.DateOnly))
	return nil
}
