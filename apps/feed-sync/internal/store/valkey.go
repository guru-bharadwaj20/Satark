// Package store holds the sinks that persist feed snapshots.
//
// Valkey key contract (read by url-intel's threatfeed package):
//
//	satark:feed:<feed>:urls   SET of canonical URLs
//	satark:feed:<feed>:hosts  SET of hostnames (shared hosting excluded)
//	satark:feed:meta          HASH  <feed>:last_success -> unix seconds
//	                                <feed>:count        -> entries in snapshot
package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/guru-bharadwaj20/satark/apps/feed-sync/internal/feeds"
)

// Key helpers shared with tests.
const MetaKey = "satark:feed:meta"

// URLsKey returns the URL set key for feed.
func URLsKey(feed string) string { return "satark:feed:" + feed + ":urls" }

// HostsKey returns the host set key for feed.
func HostsKey(feed string) string { return "satark:feed:" + feed + ":hosts" }

const batchSize = 1000

// Valkey stores snapshots as sets, swapped in atomically with RENAME.
type Valkey struct{ C redis.UniversalClient }

// Name implements syncer.Sink.
func (*Valkey) Name() string { return "valkey" }

// Store implements syncer.Sink.
func (v *Valkey) Store(ctx context.Context, feed string, entries []feeds.Entry, at time.Time) error {
	suffix := ":tmp:" + strconv.FormatInt(at.UnixNano(), 36)
	tmpURLs, tmpHosts := URLsKey(feed)+suffix, HostsKey(feed)+suffix
	cleanup := func() { v.C.Del(context.WithoutCancel(ctx), tmpURLs, tmpHosts) }

	hostSeen := map[string]bool{}
	for start := 0; start < len(entries); start += batchSize {
		end := min(start+batchSize, len(entries))
		pipe := v.C.Pipeline()
		urls := make([]any, 0, end-start)
		var hosts []any
		for _, e := range entries[start:end] {
			urls = append(urls, e.URL)
			if e.Host != "" && !hostSeen[e.Host] && !feeds.IsSharedHost(e.Host) {
				hostSeen[e.Host] = true
				hosts = append(hosts, e.Host)
			}
		}
		pipe.SAdd(ctx, tmpURLs, urls...)
		if len(hosts) > 0 {
			pipe.SAdd(ctx, tmpHosts, hosts...)
		}
		// Temporary keys expire on their own if this process dies mid-load.
		pipe.Expire(ctx, tmpURLs, time.Hour)
		pipe.Expire(ctx, tmpHosts, time.Hour)
		if _, err := pipe.Exec(ctx); err != nil {
			cleanup()
			return fmt.Errorf("load %s: %w", feed, err)
		}
	}

	_, err := v.C.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Rename(ctx, tmpURLs, URLsKey(feed))
		p.Persist(ctx, URLsKey(feed))
		if len(hostSeen) > 0 {
			p.Rename(ctx, tmpHosts, HostsKey(feed))
			p.Persist(ctx, HostsKey(feed))
		} else {
			p.Del(ctx, HostsKey(feed))
		}
		p.HSet(ctx, MetaKey, feed+":last_success", at.Unix(), feed+":count", len(entries))
		return nil
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("swap %s: %w", feed, err)
	}
	return nil
}
