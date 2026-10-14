// Package ratelimit limits how many reports a single reporter can file,
// so one account cannot flood the community data.
package ratelimit

import (
	"sync"
	"time"
)

// Window is one limit: at most Max events per Period.
type Window struct {
	Max    int
	Period time.Duration
}

// Limiter enforces several fixed windows per key (e.g. 5/hour and 20/day).
// State is in-process; with several replicas each keeps its own counts, so
// the effective limit is per pod. Reporter keys are already hashed.
type Limiter struct {
	mu      sync.Mutex
	windows []Window
	events  map[string][]time.Time
	now     func() time.Time
	calls   int
}

// New returns a Limiter enforcing all windows.
func New(now func() time.Time, windows ...Window) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{windows: windows, events: map[string][]time.Time{}, now: now}
}

// Allow records an event for key if every window permits it. When denied
// it returns how long the caller should wait before retrying.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	longest := time.Duration(0)
	for _, w := range l.windows {
		if w.Period > longest {
			longest = w.Period
		}
	}
	ev := l.events[key]
	// Drop events older than the longest window.
	i := 0
	for i < len(ev) && now.Sub(ev[i]) >= longest {
		i++
	}
	ev = ev[i:]
	for _, w := range l.windows {
		n, oldest := 0, time.Time{}
		for _, t := range ev {
			if now.Sub(t) < w.Period {
				if n == 0 {
					oldest = t
				}
				n++
			}
		}
		if n >= w.Max {
			l.events[key] = ev
			return false, w.Period - now.Sub(oldest)
		}
	}
	l.events[key] = append(ev, now)
	l.calls++
	if l.calls%1024 == 0 {
		l.gc(now, longest)
	}
	return true, 0
}

func (l *Limiter) gc(now time.Time, longest time.Duration) {
	for k, ev := range l.events {
		if len(ev) == 0 || now.Sub(ev[len(ev)-1]) >= longest {
			delete(l.events, k)
		}
	}
}
