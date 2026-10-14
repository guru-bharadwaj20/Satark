package urlscan

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeURLIntel imitates url-intel's /v1/analyze.
func fakeURLIntel(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/analyze" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var req struct{ URL string }
		json.NewDecoder(r.Body).Decode(&req)
		switch {
		case strings.Contains(req.URL, "slow"):
			time.Sleep(200 * time.Millisecond)
		case strings.Contains(req.URL, "ftp:"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"error":"invalid input: only http and https urls are supported"}`))
			return
		case strings.Contains(req.URL, "broken"):
			w.WriteHeader(http.StatusInternalServerError)
			return
		case strings.Contains(req.URL, "phish"):
			w.Write([]byte(`{"target":{"normalized":"http://phish.example/","final_url":"http://phish.example/"},
				"signals":[{"id":"feed.url_listed","severity":"high","weight":70,"detail":"listed by openphish"},
				           {"id":"domain.very_new","severity":"high","weight":30,"detail":"registered 3 days ago"}],"score":100}`))
			return
		}
		w.Write([]byte(`{"target":{"normalized":"https://uidai.gov.in/","final_url":"https://uidai.gov.in/"},
			"signals":[{"id":"allowlist.official","severity":"info","weight":-40,"detail":"official"}],"score":0,"allowlisted":"official .gov.in domain"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func do(h http.Handler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/scan/url", strings.NewReader(body)))
	return rec
}

func TestHandler(t *testing.T) {
	srv := fakeURLIntel(t)
	h := NewHandler(NewClient(srv.URL, time.Second))
	h.Timeout = 100 * time.Millisecond

	rec := do(h, `{"url":"http://phish.example"}`)
	var resp Response
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != 200 || resp.Verdict != VerdictDangerous || len(resp.Reasons) != 2 || resp.Reasons[0] != "listed by openphish" {
		t.Fatalf("%d %+v", rec.Code, resp)
	}

	rec = do(h, `{"url":"https://uidai.gov.in"}`)
	resp = Response{}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Verdict != VerdictNoRedFlags || !resp.Official || resp.Headline != "No red flags found" {
		t.Fatalf("%+v", resp)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "100% safe") {
		t.Fatal("must never claim a link is 100% safe")
	}

	for body, code := range map[string]int{
		`{"url":"ftp://x.example"}`:       http.StatusUnprocessableEntity,
		`{"url":"http://broken.example"}`: http.StatusBadGateway,
		`{"url":"http://slow.example"}`:   http.StatusGatewayTimeout,
		`{"url":""}`:                      http.StatusBadRequest,
		`not json`:                        http.StatusBadRequest,
		`{"url":"x","extra":1}`:           http.StatusBadRequest,
	} {
		if rec := do(h, body); rec.Code != code {
			t.Errorf("%s: got %d want %d (%s)", body, rec.Code, code, rec.Body)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/scan/url", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatal(rec.Code)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		rep  Report
		want string
	}{
		{Report{Score: 10}, VerdictNoRedFlags},
		{Report{Score: 30}, VerdictSuspicious},
		{Report{Score: 75}, VerdictDangerous},
		// A feed hit on an allowlisted site is still dangerous.
		{Report{Score: 30, Signals: []Signal{{Severity: "high", Weight: 70}}}, VerdictDangerous},
	}
	for i, c := range cases {
		if got := Classify(&c.rep); got != c.want {
			t.Errorf("%d: got %s want %s", i, got, c.want)
		}
	}
}
