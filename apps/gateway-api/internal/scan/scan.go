// Package scan orchestrates a scan: it fans out to the analyzer services
// concurrently, each under its own deadline inside an overall scan
// deadline, and turns whatever came back in time into a ScanResponse.
package scan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/analyzers"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/api"
	"github.com/guru-bharadwaj20/satark/apps/gateway-api/internal/server"
)

var analyzerCalls = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "satark_gateway_analyzer_call_duration_seconds",
	Help:    "Latency of calls from gateway-api to analyzer services.",
	Buckets: []float64{.01, .025, .05, .1, .25, .5, 1, 2, 4},
}, []string{"analyzer", "outcome"})

// TextAnalyzer is implemented by analyzers.TextAnalyzer.
type TextAnalyzer interface {
	Analyze(ctx context.Context, text, lang string) (analyzers.TextResult, error)
}

// QRDecoder is implemented by analyzers.QRDecoder.
type QRDecoder interface {
	Decode(ctx context.Context, image []byte, caption string) (analyzers.QRResult, error)
}

// Findings is everything the analyzers reported for one scan.
type Findings struct {
	Text       *analyzers.TextResult
	QR         *analyzers.QRResult
	SenderType string
	Failed     []string // analyzers that errored or timed out
}

// Scorer turns findings into a scored response.
type Scorer interface {
	Score(f Findings, lang string) api.ScanResponse
}

// Service implements server.Scanner.
type Service struct {
	Text    TextAnalyzer
	QR      QRDecoder
	Scorer  Scorer
	Timeout time.Duration // overall deadline for one scan
	Log     *slog.Logger
}

var _ server.Scanner = (*Service)(nil)

// call runs fn and records its latency and outcome.
func call(ctx context.Context, name string, fn func(context.Context) error) error {
	start := time.Now()
	err := fn(ctx)
	outcome := "ok"
	switch {
	case err == nil:
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	case errors.Is(err, analyzers.ErrBadInput):
		outcome = "bad_input"
	default:
		outcome = "error"
	}
	analyzerCalls.WithLabelValues(name, outcome).Observe(time.Since(start).Seconds())
	return err
}

type task struct {
	name string
	run  func(context.Context) error
}

// fanOut runs tasks concurrently and returns the names of those that failed.
func (s *Service) fanOut(ctx context.Context, tasks []task) []string {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed []string
	)
	for _, t := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := call(ctx, t.name, t.run); err != nil {
				s.Log.Warn("analyzer failed", "analyzer", t.name, "err", err,
					"request_id", server.RequestID(ctx))
				mu.Lock()
				failed = append(failed, t.name)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return failed
}

// ScanText implements server.Scanner.
func (s *Service) ScanText(ctx context.Context, req api.TextScanRequest) (api.ScanResponse, error) {
	ctx = analyzers.WithRequestID(ctx, server.RequestID(ctx))
	f := Findings{SenderType: req.SenderType}
	var text analyzers.TextResult
	f.Failed = s.fanOut(ctx, []task{
		{"text-analyzer", func(ctx context.Context) (err error) {
			text, err = s.Text.Analyze(ctx, req.Text, req.Lang)
			return err
		}},
	})
	if ctx.Err() != nil {
		return api.ScanResponse{}, ctx.Err()
	}
	if len(f.Failed) == 0 {
		f.Text = &text
	}
	// Without the text analyzer there is no redaction and no rule data, so
	// returning a verdict would be misleading.
	if f.Text == nil {
		return api.ScanResponse{}, server.ErrUnavailable
	}
	return s.finish(f, req.Lang), nil
}

// ScanQR implements server.Scanner. When the QR came with a caption, the
// caption is analyzed in parallel with decoding.
func (s *Service) ScanQR(ctx context.Context, req api.QRScanRequest) (api.ScanResponse, error) {
	ctx = analyzers.WithRequestID(ctx, server.RequestID(ctx))
	f := Findings{SenderType: api.SenderUnknown}
	var (
		qr   analyzers.QRResult
		text analyzers.TextResult
		qrOK bool
		txOK bool
	)
	var qrErr error
	tasks := []task{{"qr-decoder", func(ctx context.Context) error {
		qr, qrErr = s.QR.Decode(ctx, req.Image, req.Context)
		qrOK = qrErr == nil
		return qrErr
	}}}
	if req.Context != "" {
		tasks = append(tasks, task{"text-analyzer", func(ctx context.Context) error {
			var err error
			text, err = s.Text.Analyze(ctx, req.Context, req.Lang)
			txOK = err == nil
			return err
		}})
	}
	f.Failed = s.fanOut(ctx, tasks)
	if ctx.Err() != nil {
		return api.ScanResponse{}, ctx.Err()
	}
	if errors.Is(qrErr, analyzers.ErrBadInput) {
		return api.ScanResponse{}, server.ErrBadImage
	}
	if !qrOK {
		return api.ScanResponse{}, server.ErrUnavailable
	}
	f.QR = &qr
	if txOK {
		f.Text = &text
	}
	return s.finish(f, req.Lang), nil
}

func (s *Service) finish(f Findings, lang string) api.ScanResponse {
	resp := s.Scorer.Score(f, lang)
	resp.ScanID = newScanID()
	resp.Lang = lang
	resp.Partial = len(f.Failed) > 0
	resp.Disclaimer = api.Disclaimer
	if f.Text != nil && f.QR == nil {
		resp.RedactedText = f.Text.RedactedText
	}
	if f.QR != nil {
		for _, c := range f.QR.Codes {
			r := api.QRResult{Kind: c.Kind, Format: c.Format}
			switch c.Kind {
			case "upi":
				if c.UPI != nil {
					r.UPI = &api.UPIIntent{
						PayeeAddress: c.UPI.PayeeAddress, PayeeName: c.UPI.PayeeName,
						Amount: c.UPI.Amount, Currency: c.UPI.Currency, Note: c.UPI.Note,
						Direction: c.UPI.Direction, Summary: c.UPI.Summary,
					}
				}
			case "url":
				r.URL = c.Text
			}
			resp.QR = append(resp.QR, r)
		}
	}
	if resp.Reasons == nil {
		resp.Reasons = []api.Reason{}
	}
	if resp.NextSteps == nil {
		resp.NextSteps = []string{}
	}
	return resp
}

func newScanID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
