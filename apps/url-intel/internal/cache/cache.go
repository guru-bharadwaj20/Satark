// Package cache memoises url-intel reports in Valkey with a TTL so viral
// scam links (the same URL forwarded thousands of times) are analysed once.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/analyzer"
	"github.com/guru-bharadwaj20/satark/apps/url-intel/internal/normalize"
)

// keyPrefix is versioned so a report format change can invalidate old entries.
const keyPrefix = "satark:urlintel:v1:"

// Inner is the analyzer being cached.
type Inner interface {
	Analyze(ctx context.Context, raw string) (*analyzer.Report, error)
}

// Analyzer is a caching decorator.
type Analyzer struct {
	Next Inner
	C    redis.UniversalClient
	// TTL for reports; risky reports are kept longer (RiskyTTL) because
	// they rarely become safe, while clean ones may turn bad.
	TTL       time.Duration
	RiskyTTL  time.Duration
	RiskyFrom int
	Log       *slog.Logger

	group  singleflight.Group
	lookup *prometheus.CounterVec
}

// New returns a caching Analyzer and registers its metrics on reg.
func New(next Inner, c redis.UniversalClient, ttl, riskyTTL time.Duration, log *slog.Logger, reg prometheus.Registerer) *Analyzer {
	a := &Analyzer{
		Next: next, C: c, TTL: ttl, RiskyTTL: riskyTTL, RiskyFrom: 50, Log: log,
		lookup: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "satark_url_intel_cache_lookups_total", Help: "Report cache lookups by result (hit, miss, error).",
		}, []string{"result"}),
	}
	if reg != nil {
		reg.MustRegister(a.lookup)
	}
	return a
}

// Key returns the cache key for a canonical URL.
func Key(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return keyPrefix + hex.EncodeToString(sum[:])
}

// Analyze returns a cached report when present, otherwise analyses and
// stores it. Cache failures never fail the request.
func (a *Analyzer) Analyze(ctx context.Context, raw string) (*analyzer.Report, error) {
	n, err := normalize.Normalize(raw)
	if err != nil {
		return nil, err
	}
	key := Key(n.URL.String())

	if b, err := a.C.Get(ctx, key).Bytes(); err == nil {
		var rep analyzer.Report
		if json.Unmarshal(b, &rep) == nil {
			a.lookup.WithLabelValues("hit").Inc()
			return &rep, nil
		}
	} else if errors.Is(err, redis.Nil) {
		a.lookup.WithLabelValues("miss").Inc()
	} else {
		a.lookup.WithLabelValues("error").Inc()
		a.Log.Warn("cache get failed", "err", err)
	}

	// Collapse concurrent requests for the same URL into one analysis.
	v, err, _ := a.group.Do(key, func() (any, error) {
		rep, err := a.Next.Analyze(ctx, raw)
		if err != nil {
			return nil, err
		}
		a.store(context.WithoutCancel(ctx), key, rep)
		return rep, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*analyzer.Report), nil
}

func (a *Analyzer) store(ctx context.Context, key string, rep *analyzer.Report) {
	// Reports where a lookup errored are cached briefly so a transient
	// outage does not pin an incomplete verdict for the full TTL.
	ttl := a.TTL
	if rep.Score >= a.RiskyFrom && a.RiskyTTL > 0 {
		ttl = a.RiskyTTL
	}
	for _, f := range rep.Findings {
		if f.Error != "" && ttl > 5*time.Minute {
			ttl = 5 * time.Minute
			break
		}
	}
	b, err := json.Marshal(rep)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := a.C.Set(ctx, key, b, ttl).Err(); err != nil {
		a.Log.Warn("cache set failed", "err", err)
	}
}
