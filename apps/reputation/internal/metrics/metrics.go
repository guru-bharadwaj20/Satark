// Package metrics is a tiny dependency-free Prometheus text exposition
// for labelled counters (metric prefix satark_).
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Registry holds counters keyed by name and label set.
type Registry struct {
	mu   sync.Mutex
	help map[string]string
	vals map[string]map[string]float64
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{help: map[string]string{}, vals: map[string]map[string]float64{}}
}

// Describe registers a counter with its help text.
func (r *Registry) Describe(name, help string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.help[name] = help
	if r.vals[name] == nil {
		r.vals[name] = map[string]float64{}
	}
}

// Inc increments name with the given label pairs ("k1","v1","k2","v2").
func (r *Registry) Inc(name string, labels ...string) {
	var b strings.Builder
	for i := 0; i+1 < len(labels); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%q", labels[i], labels[i+1])
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.vals[name] == nil {
		r.vals[name] = map[string]float64{}
	}
	r.vals[name][b.String()]++
}

// ServeHTTP writes all counters in Prometheus text format.
func (r *Registry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	names := make([]string, 0, len(r.vals))
	for n := range r.vals {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n", n, r.help[n], n)
		keys := make([]string, 0, len(r.vals[n]))
		for k := range r.vals[n] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "" {
				fmt.Fprintf(w, "%s %g\n", n, r.vals[n][k])
			} else {
				fmt.Fprintf(w, "%s{%s} %g\n", n, k, r.vals[n][k])
			}
		}
	}
}
