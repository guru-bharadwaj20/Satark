package scan

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/store"
)

type fakeScanner struct {
	resp api.ScanResponse
	err  error
}

func (f fakeScanner) ScanText(context.Context, api.TextScanRequest) (api.ScanResponse, error) {
	return f.resp, f.err
}

func (f fakeScanner) ScanQR(context.Context, api.QRScanRequest) (api.ScanResponse, error) {
	return f.resp, f.err
}

type fakeSaver struct {
	saved []store.Scan
	err   error
}

func (f *fakeSaver) SaveScan(_ context.Context, sc store.Scan) (string, error) {
	f.saved = append(f.saved, sc)
	return "id", f.err
}

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRecorderSavesTextScan(t *testing.T) {
	resp := api.ScanResponse{Verdict: api.VerdictSuspicious, RiskScore: 55, Lang: "en",
		Reasons: []api.Reason{{Code: "urgency"}, {Code: "personal_number"}}}
	sv := &fakeSaver{}
	r := &Recorder{Next: fakeScanner{resp: resp}, Store: sv, Log: quietLog()}

	got, err := r.ScanText(context.Background(), api.TextScanRequest{Text: "pay now", Lang: "en"})
	if err != nil || got.Verdict != api.VerdictSuspicious {
		t.Fatalf("ScanText = %+v, %v", got, err)
	}
	if len(sv.saved) != 1 {
		t.Fatalf("saved %d scans, want 1", len(sv.saved))
	}
	s := sv.saved[0]
	if s.Kind != store.KindText || s.Input != "pay now" || s.RiskScore != 55 ||
		s.Verdict != store.VerdictSuspicious || len(s.Reasons) != 2 || s.Reasons[0] != "urgency" {
		t.Fatalf("saved %+v", s)
	}
}

func TestRecorderKeepsVerdictWhenSaveFails(t *testing.T) {
	sv := &fakeSaver{err: errors.New("db down")}
	r := &Recorder{Next: fakeScanner{resp: api.ScanResponse{Verdict: api.VerdictNoRedFlags}}, Store: sv, Log: quietLog()}
	got, err := r.ScanText(context.Background(), api.TextScanRequest{Text: "hi"})
	if err != nil || got.Verdict != api.VerdictNoRedFlags {
		t.Fatalf("ScanText = %+v, %v", got, err)
	}
}

func TestRecorderSkipsFailedScans(t *testing.T) {
	sv := &fakeSaver{}
	r := &Recorder{Next: fakeScanner{err: errors.New("unavailable")}, Store: sv, Log: quietLog()}
	if _, err := r.ScanQR(context.Background(), api.QRScanRequest{}); err == nil {
		t.Fatal("want error")
	}
	if len(sv.saved) != 0 {
		t.Fatalf("saved %d scans, want 0", len(sv.saved))
	}
}

func TestRecorderHashesQRPayloadNotImage(t *testing.T) {
	resp := api.ScanResponse{Verdict: api.VerdictLikelyScam, QR: []api.QRResult{
		{Kind: "upi", UPI: &api.UPIIntent{PayeeAddress: "xy@bank"}},
		{Kind: "url", URL: "https://example.test/x"},
	}}
	sv := &fakeSaver{}
	r := &Recorder{Next: fakeScanner{resp: resp}, Store: sv, Log: quietLog()}
	if _, err := r.ScanQR(context.Background(), api.QRScanRequest{Image: []byte("PNGDATA")}); err != nil {
		t.Fatal(err)
	}
	if got := sv.saved[0].Input; got != "upi:xy@bank\nhttps://example.test/x" {
		t.Fatalf("input = %q", got)
	}
}
