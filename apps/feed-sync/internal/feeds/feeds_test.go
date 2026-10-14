package feeds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseLinesFixture(t *testing.T) {
	f, err := os.Open("testdata/urlhaus_text_online.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := ParseLines(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 11 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}
	want := map[string]string{
		"https://files.bad-cdn.example/kyc/update.apk": "files.bad-cdn.example",
		"http://drop.example.test/payload.exe":         "drop.example.test",
		"http://upper.example/Path":                    "upper.example",
		"http://198.51.100.23:8080/i":                  "198.51.100.23",
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.URL] = e.Host
	}
	for u, h := range want {
		if got[u] != h {
			t.Errorf("%s: host %q", u, got[u])
		}
	}
}

func TestFetchRetriesAndGuards(t *testing.T) {
	var calls atomic.Int32
	body := strings.Repeat("http://phish.example/x\n", 1)
	for i := 0; i < 20; i++ {
		body += "http://phish" + string(rune('a'+i)) + ".example/login\n"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/flaky":
			if calls.Add(1) < 3 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.Write([]byte(body))
		case "/tiny":
			w.Write([]byte("http://one.example/\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fe := NewFetcher(time.Second)
	fe.Backoff = time.Millisecond
	entries, err := fe.Fetch(context.Background(), Feed{Name: "flaky", URL: srv.URL + "/flaky"})
	if err != nil || len(entries) != 21 {
		t.Fatalf("%d %v", len(entries), err)
	}
	if _, err := fe.Fetch(context.Background(), Feed{Name: "tiny", URL: srv.URL + "/tiny"}); err == nil {
		t.Fatal("expected too-few error")
	}
	calls.Store(0)
	if _, err := fe.Fetch(context.Background(), Feed{Name: "gone", URL: srv.URL + "/gone"}); err == nil {
		t.Fatal("expected 404 error")
	}
}
