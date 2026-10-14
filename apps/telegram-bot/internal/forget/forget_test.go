package forget

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequiresConfirmation(t *testing.T) {
	called := false
	c := New(FuncEraser{"settings", func(context.Context, string) error { called = true; return nil }})
	r := c.Start("tg:1")
	if called || len(r.Buttons) == 0 {
		t.Fatal("Start must only ask for confirmation")
	}
	if r := c.Callback(context.Background(), "tg:1", CancelData); called || !strings.Contains(r.Text, "Nothing was deleted") {
		t.Fatal("cancel must not delete")
	}
	// Confirming without a pending /forget does nothing.
	if r := c.Callback(context.Background(), "tg:1", ConfirmData); called || !strings.Contains(r.Text, "expired") {
		t.Fatalf("stale confirm: %q", r.Text)
	}
}

func TestExpiry(t *testing.T) {
	now := time.Now()
	c := New(FuncEraser{"x", func(context.Context, string) error { t.Fatal("ran"); return nil }})
	c.Now = func() time.Time { return now }
	c.Start("tg:1")
	now = now.Add(6 * time.Minute)
	c.Callback(context.Background(), "tg:1", ConfirmData)
}

func TestPartialFailure(t *testing.T) {
	var got []string
	ok := FuncEraser{"settings", func(_ context.Context, id string) error { got = append(got, id); return nil }}
	bad := FuncEraser{"scan history", func(context.Context, string) error { return errors.New("down") }}
	c := New(ok, bad)
	c.Start("tg:7")
	r := c.Callback(context.Background(), "tg:7", ConfirmData)
	if len(got) != 1 || got[0] != "tg:7" || !strings.Contains(r.Text, "scan history") {
		t.Fatalf("got %v, reply %q", got, r.Text)
	}
}

func TestHTTPEraser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if string(b) != `{"reporter_id":"tg:1"}` {
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()
	e := NewHTTPEraser("community reports", srv.URL, "reporter_id")
	if err := e.Forget(context.Background(), "tg:1"); err != nil {
		t.Fatal(err)
	}
	if err := e.Forget(context.Background(), "tg:2"); err == nil {
		t.Fatal("want error on 400")
	}
}
