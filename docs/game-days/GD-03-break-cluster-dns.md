# GD-03: Break in-cluster DNS

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
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

| Measure | Variant A | Variant B |
| --- | --- | --- |
| Time to alert | | |
| Time to diagnose | | |
| Probe failures | | |
| User-facing behaviour | | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
