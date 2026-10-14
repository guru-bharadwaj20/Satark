package expand

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
)

// testExpander allows loopback test servers, which production rejects.
func testExpander(max int) *Expander {
	e := New(2*time.Second, max)
	e.Client.Transport = http.DefaultTransport
	e.Validate = url.Parse
	return e
}

func chainServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/hop/{n}", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.PathValue("n"))
		if n == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n-1), http.StatusFound)
	})
	mux.HandleFunc("/nohead", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.Redirect(w, r, "/hop/0", http.StatusMovedPermanently)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestExpandFollowsChain(t *testing.T) {
	srv := chainServer(t)
	start, _ := url.Parse(srv.URL + "/hop/3")
	res := testExpander(5).Expand(context.Background(), start)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if len(res.Chain) != 4 || res.Final.Path != "/hop/0" {
		t.Fatalf("unexpected chain %v", res.Chain)
	}
}

func TestExpandBounded(t *testing.T) {
	srv := chainServer(t)
	start, _ := url.Parse(srv.URL + "/hop/10")
	res := testExpander(2).Expand(context.Background(), start)
	if res.Err != ErrTooManyRedirects {
		t.Fatalf("expected limit error, got %v", res.Err)
	}
	if len(res.Chain) != 3 {
		t.Fatalf("chain length %d", len(res.Chain))
	}
}

func TestExpandFallsBackToGet(t *testing.T) {
	srv := chainServer(t)
	start, _ := url.Parse(srv.URL + "/nohead")
	res := testExpander(5).Expand(context.Background(), start)
	if res.Err != nil || res.Final.Path != "/hop/0" {
		t.Fatalf("got %v %v", res.Final, res.Err)
	}
}

func TestProductionClientBlocksPrivate(t *testing.T) {
	srv := chainServer(t)
	e := New(time.Second, 3)
	start, _ := url.Parse(srv.URL + "/hop/0")
	if res := e.Expand(context.Background(), start); res.Err == nil {
		t.Fatal("expected loopback dial to be refused")
	}
}

func TestPreparerSignals(t *testing.T) {
	srv := chainServer(t)
	u, _ := url.Parse(srv.URL + "/hop/4")
	tgt := &analyzer.Target{URL: u}
	sig, err := Preparer{E: testExpander(3)}.Prepare(context.Background(), tgt)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, s := range sig {
		ids[s.ID] = true
	}
	if !ids["redirect.limit_exceeded"] || !ids["redirect.many_hops"] {
		t.Fatalf("signals: %+v", sig)
	}
	if len(tgt.RedirectChain) != 4 {
		t.Fatalf("chain %v", tgt.RedirectChain)
	}
}

func TestDowngraded(t *testing.T) {
	if !downgraded([]string{"https://a.example/", "http://b.example/"}) {
		t.Fatal("expected downgrade")
	}
	if downgraded([]string{"http://a.example/", "https://b.example/"}) {
		t.Fatal("upgrade flagged as downgrade")
	}
}
