// Package cache stores scan verdicts in Valkey, keyed by the HMAC digest of
// the normalized input, so repeated scans of the same forwarded message or
// link are answered without re-running the analyzers.
//
// Keys look like "satark:verdict:v1:<kind>:<hex digest>"; values are JSON.
// The raw input is never part of the key or value. Cache failures are
// treated as misses: Valkey being down must slow Satark, not break it.
//
// Wiring (gateway-api main), requires github.com/redis/go-redis/v9:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "valkey.satark-data:6379",
//	    Password: os.Getenv("VALKEY_PASSWORD")})
//	vc := cache.New(rdb, cache.Options{})
//	key := cache.Key("text", digest)            // digest from hashing.Hasher.Input
//	if v, ok := vc.Get(ctx, key); ok { return v }
//	... run analyzers ...
//	vc.Set(ctx, key, verdict)
package cache

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "satark:verdict:v1:"

// Verdict is the cached result of a scan.
type Verdict struct {
	RiskScore int       `json:"risk_score"`
	Verdict   string    `json:"verdict"`
	Reasons   []string  `json:"reasons"`
	ScanID    string    `json:"scan_id,omitempty"`
	CachedAt  time.Time `json:"cached_at"`
}

// Options configures the cache. Zero values pick defaults.
type Options struct {
	// TTL for verdicts; likely-scam verdicts are kept for LongTTL because
	// scam content rarely becomes legitimate.
	TTL     time.Duration // default 6h
	LongTTL time.Duration // default 24h
	Timeout time.Duration // per-call timeout, default 150ms
	Logger  *slog.Logger
}

// Cache is a Valkey-backed verdict cache.
type Cache struct {
	rdb  redis.Cmdable
	opts Options
}

// New returns a Cache using any go-redis client (Client, ClusterClient, Ring).
func New(rdb redis.Cmdable, o Options) *Cache {
	if o.TTL <= 0 {
		o.TTL = 6 * time.Hour
	}
	if o.LongTTL <= 0 {
		o.LongTTL = 24 * time.Hour
	}
	if o.Timeout <= 0 {
		o.Timeout = 150 * time.Millisecond
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Cache{rdb: rdb, opts: o}
}

// Key builds the cache key for an input kind and its HMAC digest.
func Key(kind string, digest []byte) string {
	return keyPrefix + kind + ":" + hex.EncodeToString(digest)
}

// Get returns a cached verdict. Any error is logged and reported as a miss.
func (c *Cache) Get(ctx context.Context, key string) (Verdict, bool) {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			c.opts.Logger.WarnContext(ctx, "verdict cache get failed", "err", err)
		}
		return Verdict{}, false
	}
	var v Verdict
	if err := json.Unmarshal(raw, &v); err != nil {
		c.opts.Logger.WarnContext(ctx, "verdict cache entry corrupt", "err", err)
		_ = c.rdb.Del(ctx, key).Err()
		return Verdict{}, false
	}
	return v, true
}

// Set stores a verdict. Errors are logged and returned for callers that care.
func (c *Cache) Set(ctx context.Context, key string, v Verdict) error {
	if v.CachedAt.IsZero() {
		v.CachedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ttl := c.opts.TTL
	if v.Verdict == "likely_scam" {
		ttl = c.opts.LongTTL
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	if err := c.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		c.opts.Logger.WarnContext(ctx, "verdict cache set failed", "err", err)
		return err
	}
	return nil
}

// Invalidate removes a verdict, e.g. after feedback marks it wrong.
func (c *Cache) Invalidate(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	return c.rdb.Del(ctx, key).Err()
}
