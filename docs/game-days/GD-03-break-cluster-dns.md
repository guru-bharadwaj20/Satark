# GD-03: Break in-cluster DNS

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-03-break-dns.sh` |
| Expected duration | 60 minutes (two variants) |

## Hypothesis

**Variant A, CoreDNS scaled to 0:**
- Every service-to-service call by name fails within the resolver cache TTL
  (Go services: immediately; Python: per request). `gateway-api` returns 5xx
  for checks that fan out to `text-analyzer` and `url-intel`.
- Existing Postgres connections keep working (already resolved); new ones fail.
- `CoreDNSDown` / `KubeDeploymentReplicasMismatch` alerts fire within 2 minutes.

**Variant B, broken upstream forwarder:**
- `*.cluster.local` resolves; external names do not. Link checks
  (`url-intel` redirect expansion, RDAP, threat feeds) time out and the API
  returns a degraded "could not verify" verdict instead of a 5xx.
- The Telegram bot cannot reach `api.telegram.org`.

This is the harder variant: the cluster looks healthy, so it tests whether
the responder checks DNS early.

## Blast radius and abort criteria

- Scope: whole cluster. Abort if the probe fails for more than 10 minutes.
- Abort action: `tests/chaos/gd-03-break-dns.sh revert`.

## Procedure

1. `... observe` (baseline: both lookups succeed).
2. `MODE=scale ... inject`; responder diagnoses; `... revert`.
3. Wait 10 minutes. `MODE=config ... inject`; responder diagnoses; `... revert`.
4. Useful responder commands:
   `kubectl -n kube-system get pods -l k8s-app=kube-dns`,
   `kubectl -n kube-system logs deploy/coredns`, Hubble:
   `hubble observe --protocol dns --verdict DROPPED`.

## Results

Lab: kubeadm v1.35 on three VirtualBox VMs, two CoreDNS replicas, Cilium
kube-proxy replacement. The probe sent `POST /v1/scan/text` through the
gateway once a second. That path needs `gateway-api` to reach `text-analyzer`
by name. The lab has no `url-intel`, Postgres client in the gateway, or
Prometheus, so URL checks and alert timing were not tested.

| Measure | Variant A (scale to 0) | Variant B (bad forwarder) |
| --- | --- | --- |
| Time to alert | n/a (no Prometheus in the lab) | n/a |
| Time to diagnose | under 1 min: gateway log says `lookup text-analyzer on 10.96.0.10:53 ... connect: operation not permitted` | under 1 min: CoreDNS log `read udp ...->192.0.2.1:53: i/o timeout` |
| Probe failures | every text scan failed from about 35 s after inject until 2 s after revert (149 × 503 in the trace) | 0 / 300 |
| User-facing behaviour | instant `503` (0–2 ms), no hang | none on text scans; external lookups from pods time out |

The variant A probe count in the log (282/300) also includes 429s. A second
trace probe ran from the same client IP, so the two together exceeded the
1 rps / burst 30 rate limit. The trace file (`logs/gd-03-break-dns-scale-trace-20261018.log`)
has the per-second codes. For variant B the script's own probe wrote the
trace (`PROBE_TRACE`), so there was only one client.

Caveat: the Windows host was running heavy WSL jobs alongside the lab VMs.

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 14:17:47 | A: `coredns` scaled to 0 |
| 14:17:56 | First 503 (CoreDNS `lameduck 5s`, then the pods go) |
| 14:17:56 → 14:18:26 | Mixed 200 / 503: requests that reuse a pooled connection to `text-analyzer` still work |
| 14:18:26 → 14:23:07 | Every text scan returns 503 |
| 14:23:05 | Revert: ConfigMap restored, replicas back to 2 |
| 14:23:09 | First 200; no failures after |
| 14:25:43 | B: `forward . 192.0.2.1`, CoreDNS restarted |
| 14:26:45 | In a pod: `kubernetes.default.svc.cluster.local` resolves, `example.com` times out (`no servers could be reached`) |
| 14:31:15 | Revert; external lookups work again |

## What we learned

- **Variant A** behaved as predicted, with two refinements. Failure was not
  immediate: Go's HTTP client reuses idle keep-alive connections, so calls
  on an existing connection skip DNS. Failure ramped in over ~30 s. Once it
  was total, the API failed fast: Cilium rejects a connect to a Service
  with no endpoints at once (`operation not permitted`), so clients got a
  503 in milliseconds instead of hanging. Recovery was within 2 s of
  CoreDNS being Ready.
- The rate limiter kept working through the outage. Its Valkey connection
  was already open, so it never needed a lookup.
- **Variant B** was invisible on the text path, as predicted: nothing on
  it leaves the cluster. The Telegram bot logged no `getUpdates`
  errors either. Its long-poll connection to `api.telegram.org` was already
  open, so it would only break on reconnect or a pod restart. RB-005 said
  outbound Telegram "fails"; the truth is that it fails later.
- CoreDNS logs are noisy with `envoy-gateway.` AAAA lookups going upstream:
  an ndots/search-path side effect. It is harmless here, but under variant B
  it adds 2 s timeouts for every such lookup.
- The revert in the script applied the saved ConfigMap with its old
  `resourceVersion`. That would conflict after any edit, so it is now
  stripped when the backup is taken.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| Partial failure while keep-alive connections last can mislead triage | RB-005 now explains it and names the gateway log line | guru-bharadwaj20 | ✅ |
| Revert could conflict on `resourceVersion` | `gd-03-break-dns.sh` strips it from the backup | guru-bharadwaj20 | ✅ |
| Probe gave only a count, not a timeline | `lib.sh` `probe` writes per-second codes to `$PROBE_TRACE` | guru-bharadwaj20 | ✅ |
| Alert timing (`SatarkCoreDNSDown` 3 m, `SatarkDNSServfailHigh` 10 m) untested | Re-run both variants on the cloud cluster with Prometheus | guru-bharadwaj20 | |
