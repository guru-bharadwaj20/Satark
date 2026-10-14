# Service instrumentation

Every Satark service uses one of two shared libraries so that metric names,
labels and log fields are identical across languages. Dashboards, SLO rules
and alerts depend on these names; do not hand-roll metrics in a service.

| Language | Library | Import |
|----------|---------|--------|
| Go | `libs/go/observability` | `github.com/guru-bharadwaj20/satark/libs/go/observability` |
| Python | `libs/python/satark_obs` | `satark_obs` |

## RED metrics (task 6.4)

| Series | Type | Labels |
|--------|------|--------|
| `satark_http_requests_total` | counter | `service`, `method`, `route`, `code` |
| `satark_http_request_duration_seconds` | histogram | `service`, `method`, `route` |
| `satark_http_requests_in_flight` | gauge | `service` |

`route` is always the route **template** (`/v1/scan/url`, `/v1/analyze/{lang}`),
never the raw path, to keep cardinality bounded. Unmatched paths are reported
as `route="unmatched"`.

Histogram buckets (seconds): `0.005 0.01 0.025 0.05 0.1 0.25 0.5 0.8 1 2 5 10`.
The `0.8` bucket exists so the p95 < 800 ms latency SLO can be computed exactly.

### Go

In the service's `go.mod`:

```
require github.com/guru-bharadwaj20/satark/libs/go/observability v0.0.0
replace github.com/guru-bharadwaj20/satark/libs/go/observability => ../../libs/go/observability
```

```go
reg := observability.NewRegistry()
httpm := observability.NewHTTPMetrics(reg, "gateway-api")

mux := http.NewServeMux()
httpm.Mount(mux, map[string]observability.ReadinessCheck{
    "postgres": func(ctx context.Context) error { return pool.Ping(ctx) },
})
mux.Handle("POST /v1/scan/url", httpm.Middleware("/v1/scan/url", scanURLHandler))
```

With routers that only know the template after matching (chi), call
`httpm.Observe(method, chi.RouteContext(ctx).RoutePattern(), status, elapsed)`
from a router middleware instead.

### Python (FastAPI)

Add `satark-obs` to the service's dependencies. Locally install it with
`pip install -e ../../libs/python/satark_obs`; container builds use the repo
root as build context, `COPY libs/python/satark_obs /tmp/satark_obs` and
`pip install /tmp/satark_obs` before installing the service.

```python
from satark_obs import HTTPMetrics, PrometheusMiddleware, mount_endpoints

metrics = HTTPMetrics("text-analyzer")
app = FastAPI()
app.add_middleware(PrometheusMiddleware, metrics=metrics)
mount_endpoints(app, metrics, checks={"valkey": ping_valkey})
```
