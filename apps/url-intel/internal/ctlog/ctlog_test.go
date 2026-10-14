package ctlog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

func server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "new-phish.example":
			w.Write([]byte(`[{"entry_timestamp":"2026-09-29T11:00:00.123","not_before":"2026-09-29T10:00:00"},
			                 {"entry_timestamp":"2026-09-30T11:00:00","not_before":"2026-09-30T10:00:00"}]`))
		case "old.example":
			w.Write([]byte(`[{"entry_timestamp":"2015-01-01T00:00:00","not_before":"2015-01-01T00:00:00"}]`))
		default:
			w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCT(t *testing.T) {
	c := New(http.DefaultClient)
	c.Base = server(t).URL + "/"
	c.Now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }
	u, _ := url.Parse("https://login.new-phish.example/")

	f := c.Run(context.Background(), &analyzer.Target{Host: "login.new-phish.example", URL: u})
	if len(f.Signals) != 1 || f.Signals[0].ID != "ct.first_seen_recent" {
		t.Fatalf("%+v %s", f.Signals, f.Error)
	}
	if d := f.Data.(Data); d.Entries != 2 || d.FirstSeen.Day() != 29 {
		t.Fatalf("%+v", d)
	}

	if f := c.Run(context.Background(), &analyzer.Target{Host: "old.example", URL: u}); len(f.Signals) != 0 {
		t.Fatalf("%+v", f.Signals)
	}
	f = c.Run(context.Background(), &analyzer.Target{Host: "never.example", URL: u})
	if len(f.Signals) != 1 || f.Signals[0].ID != "ct.not_logged" {
		t.Fatalf("%+v", f.Signals)
	}
}
