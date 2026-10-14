// Package ratelimit implements a token-bucket rate limiter stored in Valkey.
//
// Each bucket is a hash {tokens, ts} updated atomically by a Lua script, so
// all gateway-api replicas share one limit per client. Time comes from the
// Valkey server (TIME), which avoids clock skew between pods.
//
// Wiring (gateway-api main), requires github.com/redis/go-redis/v9:
//
//	rl := ratelimit.New(rdb, ratelimit.Options{})
//	mux.Handle("/v1/", rl.Middleware(ratelimit.Limit{Rate: 1, Burst: 30}, ratelimit.ClientKey, apiHandler))
//
// ClientKey uses the X-API-Key header when present (hashed, never stored raw),
// otherwise the client IP (X-Forwarded-For set by Envoy Gateway). On Valkey
// errors the limiter fails open and logs, so an outage of the cache does not
// take the API down. Denied requests get 429 with Retry-After.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "satark:rl:v1:"

// Limit describes a bucket: Rate tokens are added per second up to Burst.
type Limit struct {
	Rate  float64
	Burst int
}

// Result of an Allow call.
type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration // zero when allowed
}

// Options configures the limiter.
type Options struct {
	Timeout time.Duration // per-call timeout, default 100ms
	Logger  *slog.Logger
}

// Limiter is a distributed token-bucket limiter.
type Limiter struct {
	rdb  redis.Scripter
	opts Options
}

// KEYS[1] bucket key; ARGV: rate (tokens/s), burst, cost.
// Returns {allowed (0/1), remaining tokens (floored), retry_after_ms}.
var script = redis.NewScript(`
local rate  = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local cost  = tonumber(ARGV[3])
local t     = redis.call('TIME')
local now   = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)

local b = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(b[1])
local ts     = tonumber(b[2])
if tokens == nil then
  tokens = burst
  ts = now
end

local elapsed = math.max(0, now - ts)
tokens = math.min(burst, tokens + elapsed * rate / 1000)

local allowed = 0
local retry = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
else
  retry = math.ceil((cost - tokens) * 1000 / rate)
end

redis.call('HSET', KEYS[1], 'tokens', tostring(tokens), 'ts', tostring(now))
-- Expire once the bucket would be full again; idle clients cost nothing.
redis.call('PEXPIRE', KEYS[1], math.ceil(burst * 1000 / rate) + 1000)
return {allowed, math.floor(tokens), retry}
`)

// New returns a Limiter backed by any go-redis client.
func New(rdb redis.Scripter, o Options) *Limiter {
	if o.Timeout <= 0 {
		o.Timeout = 100 * time.Millisecond
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Limiter{rdb: rdb, opts: o}
}

// Allow takes one token from the bucket identified by key.
func (l *Limiter) Allow(ctx context.Context, key string, lim Limit) (Result, error) {
	return l.AllowN(ctx, key, lim, 1)
}

// AllowN takes n tokens (e.g. a heavier cost for QR image scans).
func (l *Limiter) AllowN(ctx context.Context, key string, lim Limit, n int) (Result, error) {
	if lim.Rate <= 0 || lim.Burst <= 0 || n <= 0 {
		return Result{}, fmt.Errorf("ratelimit: invalid limit %+v / cost %d", lim, n)
	}
	if n > lim.Burst {
		return Result{Allowed: false, RetryAfter: time.Duration(math.MaxInt32) * time.Second}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, l.opts.Timeout)
	defer cancel()
	vals, err := script.Run(ctx, l.rdb, []string{keyPrefix + key},
		strconv.FormatFloat(lim.Rate, 'f', -1, 64), lim.Burst, n).Int64Slice()
	if err != nil {
		return Result{}, fmt.Errorf("ratelimit: %w", err)
	}
	return Result{
		Allowed:    vals[0] == 1,
		Remaining:  int(vals[1]),
		RetryAfter: time.Duration(vals[2]) * time.Millisecond,
	}, nil
}

// KeyFunc derives the bucket key for a request.
type KeyFunc func(*http.Request) string

// ClientKey keys by API key (SHA-256, truncated) or else by client IP.
func ClientKey(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		sum := sha256.Sum256([]byte(k))
		return "key:" + hex.EncodeToString(sum[:16])
	}
	return "ip:" + clientIP(r)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware enforces lim per key and sets X-RateLimit-* headers.
func (l *Limiter) Middleware(lim Limit, key KeyFunc, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, err := l.Allow(r.Context(), key(r), lim)
		if err != nil {
			l.opts.Logger.WarnContext(r.Context(), "rate limiter unavailable, failing open", "err", err)
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("X-RateLimit-Limit", strconv.Itoa(lim.Burst))
		h.Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
		if !res.Allowed {
			secs := int(math.Ceil(res.RetryAfter.Seconds()))
			if secs < 1 {
				secs = 1
			}
			h.Set("Retry-After", strconv.Itoa(secs))
			h.Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate_limited","message":"Too many requests, please retry later."}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
