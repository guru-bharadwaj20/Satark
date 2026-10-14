package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ReadinessCheck reports whether one dependency (Postgres, Valkey, NATS...) is usable.
type ReadinessCheck func(ctx context.Context) error

// Mount registers /healthz, /readyz and /metrics on mux, the three endpoints
// every Satark service must expose.
func (m *HTTPMetrics) Mount(mux *http.ServeMux, checks map[string]ReadinessCheck) {
	mux.Handle("/metrics", m.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		failed := map[string]string{}
		for name, check := range checks {
			if err := check(ctx); err != nil {
				failed[name] = err.Error()
			}
		}
		if len(failed) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unready", "failed": failed})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
