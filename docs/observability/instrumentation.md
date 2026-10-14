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

## Business metrics (task 6.5)

| Series | Type | Labels |
|--------|------|--------|
| `satark_scans_total` | counter | `service`, `kind`, `verdict`, `category`, `channel` |
| `satark_llm_calls_total` | counter | `service`, `provider`, `model`, `outcome` |
| `satark_llm_call_duration_seconds` | histogram | `service`, `provider` |
| `satark_llm_tokens_total` | counter | `service`, `provider`, `direction` |
| `satark_cache_requests_total` | counter | `service`, `cache`, `result` |

Allowed values (anything else is recorded as `other`):

- `kind`: `url`, `text`, `qr`, `image`
- `verdict`: `low_risk`, `suspicious`, `high_risk`, `unknown` (there is no "safe" verdict)
- `category`: `phishing`, `lottery_prize`, `kyc_update`, `job_offer`, `investment`,
  `loan_app`, `digital_arrest`, `sextortion`, `courier_customs`, `upi_collect`,
  `impersonation`, `malware_apk`, `none`
- `channel`: `pwa`, `telegram`, `api`
- `outcome`: `ok`, `error`, `timeout`, `skipped`; `result`: `hit`, `miss`, `error`

Record a scan **once**, in the service that produces the final verdict
(`gateway-api`), so totals are not double counted. Downstream services record
their own LLM and cache metrics. Never put user content, URLs or phone numbers
in a label.

```go
biz := observability.NewBusinessMetrics(reg, "gateway-api")
biz.Scan("url", verdict, category, "telegram")
biz.Cache("url_intel", "hit")
```

```python
biz = BusinessMetrics("text-analyzer", metrics.registry)
with biz.time_llm("anthropic", "claude-haiku") as call:
    resp = await client.messages.create(...)
    call["in_tokens"], call["out_tokens"] = resp.usage.input_tokens, resp.usage.output_tokens
biz.cache_lookup("verdict", "miss")
```

## Structured JSON logging (task 6.7)

Services log one JSON object per line to stdout. Alloy parses these lines and
promotes `level` and `service` to Loki labels; `trace_id` is kept as
structured metadata.

| Field | Meaning |
|-------|---------|
| `time` | RFC 3339 timestamp |
| `level` | `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `msg` | Short, constant message (`"request"`, `"lookup done"`) |
| `service`, `version`, `env` | From the service name, `$SATARK_VERSION`, `$SATARK_ENV` |
| `trace_id` | `X-Request-ID` of the current request (generated if absent, echoed back) |

Level comes from `$LOG_LEVEL` (default `info`).

Privacy: values under the keys `text`, `message_text`, `body`, `phone`,
`phone_number`, `upi`, `upi_id`, `email`, `token`, `password`,
`authorization`, `api_key`, `secret` and `chat_id` are replaced by
`[REDACTED]`, and Indian mobile numbers and UPI handles are masked in every
string value. The access log records the path only, never the query string.
Still, never log message content on purpose.

```go
logger := observability.NewLogger(observability.LogOptions{Service: "gateway-api"})
slog.SetDefault(logger)
handler := observability.AccessLog(logger, mux)
// inside handlers, always pass the request context:
slog.InfoContext(r.Context(), "scan complete", "verdict", v)
```

```python
from satark_obs import AccessLogMiddleware, configure_logging

log = configure_logging("text-analyzer")
app.add_middleware(AccessLogMiddleware)
log.info("scored", extra={"verdict": verdict, "category": category})
```

Run uvicorn with `--no-access-log`; `AccessLogMiddleware` replaces it.

Add `AccessLogMiddleware` after `PrometheusMiddleware` so it is the outermost
layer and the trace id covers every log line of the request.
