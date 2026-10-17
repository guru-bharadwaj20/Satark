package scan

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/server"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/store"
)

// ScanSaver is implemented by *store.Store.
type ScanSaver interface {
	SaveScan(ctx context.Context, sc store.Scan) (string, error)
}

// Recorder wraps a Scanner and persists every successful scan. A failed
// write is logged and the verdict is still returned to the user.
type Recorder struct {
	Next  server.Scanner
	Store ScanSaver
	Log   *slog.Logger
}

var _ server.Scanner = (*Recorder)(nil)

// ScanText implements server.Scanner.
func (r *Recorder) ScanText(ctx context.Context, req api.TextScanRequest) (api.ScanResponse, error) {
	start := time.Now()
	resp, err := r.Next.ScanText(ctx, req)
	if err == nil {
		r.save(ctx, store.KindText, req.Text, resp, time.Since(start))
	}
	return resp, err
}

// ScanQR implements server.Scanner. The decoded payloads are hashed, not the
// image bytes.
func (r *Recorder) ScanQR(ctx context.Context, req api.QRScanRequest) (api.ScanResponse, error) {
	start := time.Now()
	resp, err := r.Next.ScanQR(ctx, req)
	if err == nil {
		r.save(ctx, store.KindQR, qrPayload(resp.QR), resp, time.Since(start))
	}
	return resp, err
}

func (r *Recorder) save(ctx context.Context, kind store.ScanKind, input string, resp api.ScanResponse, took time.Duration) {
	reasons := make([]string, 0, len(resp.Reasons))
	for _, rs := range resp.Reasons {
		reasons = append(reasons, rs.Code)
	}
	// Detach from the request so a client disconnect does not drop the row;
	// SaveScan bounds itself with store.WriteTimeout.
	_, err := r.Store.SaveScan(context.WithoutCancel(ctx), store.Scan{
		Kind:       kind,
		Input:      input,
		Lang:       resp.Lang,
		RiskScore:  resp.RiskScore,
		Verdict:    store.Verdict(resp.Verdict),
		Reasons:    reasons,
		AnalyzerMS: int(took.Milliseconds()),
	})
	if err != nil {
		r.Log.Warn("scan not persisted", "kind", kind, "err", err,
			"request_id", server.RequestID(ctx))
	}
}

func qrPayload(codes []api.QRResult) string {
	parts := make([]string, 0, len(codes))
	for _, c := range codes {
		switch {
		case c.URL != "":
			parts = append(parts, c.URL)
		case c.UPI != nil:
			parts = append(parts, "upi:"+c.UPI.PayeeAddress)
		default:
			parts = append(parts, c.Kind)
		}
	}
	return strings.Join(parts, "\n")
}
