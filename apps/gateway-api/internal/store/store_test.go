package store

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/hashing"
)

type fakeRow struct {
	val string
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *string:
		*d = r.val
	case *int:
		*d = 1
	}
	return nil
}

type fakeDB struct {
	sql  string
	args []any
	err  error
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.sql, f.args = sql, args
	return pgconn.NewCommandTag("INSERT 0 1"), f.err
}

func (f *fakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	f.sql, f.args = sql, args
	if _, ok := ctx.Deadline(); !ok {
		return fakeRow{err: errors.New("no deadline set")}
	}
	return fakeRow{val: "3b241101-e2bb-4255-8caf-4136c566a962", err: f.err}
}

func newStore(t *testing.T, db DB) (*Store, *hashing.Hasher) {
	t.Helper()
	h, err := hashing.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return New(db, h, nil), h
}

func TestSaveScanHashesInput(t *testing.T) {
	db := &fakeDB{}
	st, h := newStore(t, db)
	text := "Your electricity will be disconnected tonight, call 98XXXXXX21"
	id, err := st.SaveScan(context.Background(), Scan{
		Kind: KindText, Input: text, RiskScore: 91,
		Verdict: VerdictLikelyScam, Reasons: []string{"urgency"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" || !strings.Contains(db.sql, "INSERT INTO scans") {
		t.Fatalf("unexpected id %q / sql %q", id, db.sql)
	}
	want, _ := h.Input(hashing.KindText, text)
	if !bytes.Equal(db.args[1].([]byte), want) {
		t.Fatal("input_hash is not the HMAC of the input")
	}
	for _, a := range db.args {
		if s, ok := a.(string); ok && strings.Contains(s, "electricity") {
			t.Fatal("raw input must never be sent to the database")
		}
	}
	if db.args[2] != "api" || db.args[3] != "en" {
		t.Fatalf("defaults not applied: %v %v", db.args[2], db.args[3])
	}
	if string(db.args[6].([]byte)) != `["urgency"]` {
		t.Fatalf("reasons = %s", db.args[6])
	}
}

func TestSaveScanValidation(t *testing.T) {
	st, _ := newStore(t, &fakeDB{})
	bad := []Scan{
		{Kind: "voice", Verdict: VerdictSuspicious},
		{Kind: KindText, Verdict: "safe"},
		{Kind: KindText, Verdict: VerdictSuspicious, RiskScore: 101},
		{Kind: KindURL, Verdict: VerdictSuspicious, Input: "::not a url"},
	}
	for i, sc := range bad {
		if _, err := st.SaveScan(context.Background(), sc); !errors.Is(err, ErrInvalidScan) {
			t.Errorf("case %d: want ErrInvalidScan, got %v", i, err)
		}
	}
}

func TestSaveScanDBError(t *testing.T) {
	st, _ := newStore(t, &fakeDB{err: errors.New("boom")})
	_, err := st.SaveScan(context.Background(), Scan{Kind: KindQR, Input: "upi://pay?pa=x@y", Verdict: VerdictSuspicious})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want wrapped db error, got %v", err)
	}
}

func TestSaveFeedback(t *testing.T) {
	db := &fakeDB{}
	st, _ := newStore(t, db)
	if err := st.SaveFeedback(context.Background(), "id", "correct"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(db.sql, "INSERT INTO feedback") {
		t.Fatalf("sql = %q", db.sql)
	}
	if err := st.SaveFeedback(context.Background(), "id", "great"); !errors.Is(err, ErrInvalidScan) {
		t.Fatalf("want ErrInvalidScan, got %v", err)
	}
}

func TestPing(t *testing.T) {
	st, _ := newStore(t, &fakeDB{})
	if err := st.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
