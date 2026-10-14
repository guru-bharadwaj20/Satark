package safebrowsing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

func TestLookup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Client.ClientID != "satark" || len(req.ThreatInfo.ThreatTypes) != 4 {
			t.Errorf("bad request %+v", req)
		}
		for _, e := range req.ThreatInfo.ThreatEntries {
			if e.URL == "http://phish.example/login" {
				w.Write([]byte(`{"matches":[{"threatType":"SOCIAL_ENGINEERING","platformType":"ANY_PLATFORM","threat":{"url":"http://phish.example/login"}}]}`))
				return
			}
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cl := New(srv.Client(), "test-key", "test")
	cl.Endpoint = srv.URL
	c := &Check{C: cl}

	f := c.Run(context.Background(), &analyzer.Target{
		FinalURL:      "http://phish.example/login",
		RedirectChain: []string{"https://bit.ly/x", "http://phish.example/login"},
	})
	if len(f.Signals) != 1 || f.Signals[0].ID != "safebrowsing.social_engineering" {
		t.Fatalf("%+v %s", f.Signals, f.Error)
	}

	f = c.Run(context.Background(), &analyzer.Target{FinalURL: "https://ok.example/"})
	if len(f.Signals) != 0 || f.Error != "" {
		t.Fatalf("%+v", f)
	}

	cl.APIKey = "wrong"
	if f := c.Run(context.Background(), &analyzer.Target{FinalURL: "https://ok.example/"}); f.Error == "" {
		t.Fatal("expected error on 403")
	}
}

func TestDisabledWithoutKey(t *testing.T) {
	c := &Check{C: New(http.DefaultClient, "", "v")}
	if f := c.Run(context.Background(), &analyzer.Target{FinalURL: "https://x.example/"}); f.Error != "" || f.Signals != nil {
		t.Fatalf("%+v", f)
	}
}
