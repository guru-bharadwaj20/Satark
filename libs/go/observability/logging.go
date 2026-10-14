package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// Standard log fields. Every Satark log line is one JSON object with at least:
//
//	time, level, msg, service, version, env
//
// and, inside a request, trace_id. Field names match libs/python/satark_obs.
const (
	TraceHeader = "X-Request-ID"
	redacted    = "[REDACTED]"
)

// sensitiveKeys are attribute keys whose values are never logged.
var sensitiveKeys = map[string]bool{
	"text": true, "message_text": true, "body": true, "phone": true, "phone_number": true,
	"upi": true, "upi_id": true, "email": true, "token": true, "password": true,
	"authorization": true, "api_key": true, "secret": true, "chat_id": true,
}

var (
	phoneRe = regexp.MustCompile(`(?:\+?91[\s-]?)?[6-9]\d{4}[\s-]?\d{5}`)
	upiRe   = regexp.MustCompile(`[\w.\-]{2,}@[a-zA-Z]{2,}`)
)

type ctxKey struct{}

// WithTraceID stores a request/trace id in ctx for log correlation.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// TraceID returns the id stored by WithTraceID, or "".
func TraceID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// LogOptions configures NewLogger.
type LogOptions struct {
	Service string
	Version string     // defaults to $SATARK_VERSION
	Env     string     // defaults to $SATARK_ENV
	Level   slog.Level // defaults to $LOG_LEVEL (debug|info|warn|error), else info
	Output  io.Writer  // defaults to os.Stdout
}

// NewLogger returns a JSON slog.Logger with Satark's standard fields,
// trace-id injection and PII redaction. Call slog.SetDefault on the result.
func NewLogger(o LogOptions) *slog.Logger {
	if o.Output == nil {
		o.Output = os.Stdout
	}
	if o.Version == "" {
		o.Version = os.Getenv("SATARK_VERSION")
	}
	if o.Env == "" {
		o.Env = os.Getenv("SATARK_ENV")
	}
	if o.Level == 0 {
		_ = o.Level.UnmarshalText([]byte(strings.ToUpper(os.Getenv("LOG_LEVEL"))))
	}
	h := slog.NewJSONHandler(o.Output, &slog.HandlerOptions{
		Level:       o.Level,
		ReplaceAttr: redactAttr,
	})
	return slog.New(&traceHandler{h}).With(
		slog.String("service", o.Service),
		slog.String("version", o.Version),
		slog.String("env", o.Env),
	)
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if sensitiveKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, redacted)
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, MaskPII(a.Value.String()))
	}
	return a
}

// MaskPII masks Indian mobile numbers and UPI handles in free text.
func MaskPII(s string) string {
	s = phoneRe.ReplaceAllString(s, "[PHONE]")
	return upiRe.ReplaceAllString(s, "[UPI]")
}

type traceHandler struct{ slog.Handler }

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := TraceID(ctx); id != "" {
		r.AddAttrs(slog.String("trace_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &traceHandler{h.Handler.WithAttrs(as)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{h.Handler.WithGroup(name)}
}

// AccessLog propagates (or creates) X-Request-ID, stores it in the request
// context and writes one structured access-log line per request. The raw
// query string is never logged.
func AccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(TraceHeader)
		if id == "" || len(id) > 64 {
			id = newID()
		}
		w.Header().Set(TraceHeader, id)
		ctx := WithTraceID(r.Context(), id)
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rw, r.WithContext(ctx))
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
			return
		}
		level := slog.LevelInfo
		if rw.status >= 500 {
			level = slog.LevelError
		}
		logger.LogAttrs(ctx, level, "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
		)
	})
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
