package scan

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/analyzers"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/server"
)

type fakeText struct {
	delay time.Duration
	err   error
}

func (f fakeText) Analyze(ctx context.Context, text, lang string) (analyzers.TextResult, error) {
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return analyzers.TextResult{}, ctx.Err()
	}
	return analyzers.TextResult{
		RedactedText: "call [PHONE]",
		Matches:      []analyzers.RuleMatch{{ID: "otp.share", Category: "otp", Weight: 45, Reason: "asks for OTP"}},
	}, f.err
}

type fakeQR struct {
	delay time.Duration
	err   error
}

func (f fakeQR) Decode(ctx context.Context, _ []byte, _ string) (analyzers.QRResult, error) {
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return analyzers.QRResult{}, ctx.Err()
	}
	return analyzers.QRResult{Codes: []analyzers.QRCode{{
		Kind: "upi", Format: "QRCode",
		UPI:   &analyzers.QRUPI{PayeeAddress: "xy***@examplebank", Summary: "This SENDS ₹14,999"},
		Flags: []analyzers.QRFlag{{Code: "qr.receive_money", Weight: 60, Message: "m"}},
	}}}, f.err
}

func svc(text TextAnalyzer, qr QRDecoder) *Service {
	return &Service{
		Text: text, QR: qr, Scorer: EngineScorer{}, Timeout: 100 * time.Millisecond,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestScanText(t *testing.T) {
	resp, err := svc(fakeText{}, nil).ScanText(context.Background(), api.TextScanRequest{Text: "x", Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RedactedText != "call [PHONE]" || resp.RiskScore != 70 || resp.Verdict != api.VerdictLikelyScam || resp.Partial || resp.ScanID == "" {
		t.Fatalf("unexpected %+v", resp)
	}
}

func TestScanTextTimeoutIsUnavailable(t *testing.T) {
	start := time.Now()
	_, err := svc(fakeText{delay: time.Second}, nil).ScanText(context.Background(), api.TextScanRequest{Text: "x"})
	if !errors.Is(err, server.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("scan deadline not enforced")
	}
}

func TestScanQRWithCaptionRunsInParallel(t *testing.T) {
	s := svc(fakeText{delay: 60 * time.Millisecond}, fakeQR{delay: 60 * time.Millisecond})
	start := time.Now()
	resp, err := s.ScanQR(context.Background(), api.QRScanRequest{Image: []byte{1}, Context: "scan to receive", Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 100*time.Millisecond {
		t.Fatalf("analyzers did not run concurrently (%v)", el)
	}
	if resp.RiskScore != 78 || len(resp.QR) != 1 || resp.QR[0].UPI == nil || resp.RedactedText != "" {
		t.Fatalf("unexpected %+v", resp)
	}
}

func TestScanQRPartialWhenCaptionAnalyzerSlow(t *testing.T) {
	s := svc(fakeText{delay: time.Second}, fakeQR{})
	resp, err := s.ScanQR(context.Background(), api.QRScanRequest{Image: []byte{1}, Context: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Partial || resp.RiskScore != 70 {
		t.Fatalf("unexpected %+v", resp)
	}
}

func TestScanQRDecoderDown(t *testing.T) {
	_, err := svc(fakeText{}, fakeQR{err: errors.New("boom")}).ScanQR(context.Background(), api.QRScanRequest{Image: []byte{1}})
	if !errors.Is(err, server.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestHTTPClientsHonourTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	c := analyzers.TextAnalyzer{Client: analyzers.NewClient("text-analyzer", slow.URL, 50*time.Millisecond, slow.Client())}
	start := time.Now()
	_, err := c.Analyze(context.Background(), "x", "en")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("client timeout not enforced")
	}
}

func TestHTTPClientDecodesAndPropagatesRequestID(t *testing.T) {
	var gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Request-Id")
		if r.URL.Path != "/v1/analyze" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"redacted_text":"[PHONE]","redactions":{"PHONE":1},"matches":[],"lang":"en"}`))
	}))
	defer srv.Close()
	c := analyzers.TextAnalyzer{Client: analyzers.NewClient("text-analyzer", srv.URL, time.Second, srv.Client())}
	res, err := c.Analyze(analyzers.WithRequestID(context.Background(), "abc"), "x", "en")
	if err != nil || res.Redactions["PHONE"] != 1 || gotID != "abc" {
		t.Fatalf("res=%+v err=%v id=%q", res, err, gotID)
	}
}

func TestHTTPClientBadInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := analyzers.QRDecoder{Client: analyzers.NewClient("qr-decoder", srv.URL, time.Second, srv.Client())}
	if _, err := c.Decode(context.Background(), []byte{1}, ""); !errors.Is(err, analyzers.ErrBadInput) {
		t.Fatalf("got %v", err)
	}
}
