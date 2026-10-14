package report

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSubmitter struct {
	got    []string
	status string
	err    error
}

func (f *fakeSubmitter) Submit(_ context.Context, kind, value, category, reporter string) (string, error) {
	f.got = []string{kind, value, category, reporter}
	return f.status, f.err
}

func run(t *testing.T, f *Flow, inputs ...string) Reply {
	t.Helper()
	f.Start()
	var r Reply
	for _, in := range inputs {
		r = f.Handle(context.Background(), in)
	}
	return r
}

func TestHappyPathMoneyLost(t *testing.T) {
	s := &fakeSubmitter{status: "accepted"}
	f := New("tg:1", s)
	f.Now = func() time.Time { return time.Date(2026, 3, 1, 4, 30, 0, 0, time.UTC) }
	r := run(t, f, "upi", "xy***@examplebank", "upi_collect", "yes", "submit")
	if !r.Done || f.Step() != StepDone {
		t.Fatal("flow should be done")
	}
	if strings.Join(s.got, "|") != "upi|xy***@examplebank|upi_collect|tg:1" {
		t.Fatalf("submitted %v", s.got)
	}
	for _, want := range []string{"1930", "cybercrime.gov.in", "sancharsaathi.gov.in", "01 Mar 2026, 10:00 IST"} {
		if !strings.Contains(r.Text, want) {
			t.Errorf("reply missing %q:\n%s", want, r.Text)
		}
	}
	if strings.Index(r.Text, "1930") > strings.Index(r.Text, "Chakshu") {
		t.Error("1930 must come first when money was lost")
	}
}

func TestSubmitFailureStillGivesChannels(t *testing.T) {
	f := New("tg:1", &fakeSubmitter{err: errors.New("down")})
	r := run(t, f, "phone", "98XXXXXX21", "kyc_fraud", "no", "submit")
	if !strings.Contains(r.Text, "couldn't save") || !strings.Contains(r.Text, "Chakshu") {
		t.Fatalf("got %s", r.Text)
	}
	if strings.Contains(strings.ToLower(r.Text), "scammer") {
		t.Fatal("must not label anyone")
	}
}

func TestCancelAndValidation(t *testing.T) {
	s := &fakeSubmitter{}
	f := New("tg:1", s)
	if r := run(t, f, "phone", "98XXXXXX21", "bogus"); r.Text != "Please choose one of the options." {
		t.Fatalf("got %q", r.Text)
	}
	if r := f.Handle(context.Background(), "cancel"); !r.Done || s.got != nil {
		t.Fatal("cancel should end without submitting")
	}
}

func TestClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/v1/reports" || body["reporter_id"] != "tg:1" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"duplicate"}`))
	}))
	defer srv.Close()
	st, err := NewClient(srv.URL).Submit(context.Background(), "phone", "x", "other", "tg:1")
	if err != nil || st != "duplicate" {
		t.Fatalf("%q %v", st, err)
	}
}
