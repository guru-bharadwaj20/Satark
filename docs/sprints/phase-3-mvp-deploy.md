# Phase 3 MVP deployment report (task 3.18)

The MVP deployed with Helm to the on-prem lab cluster from task 1.22, then
checked with `scripts/smoke-test.sh`.

| Field | Value |
|---|---|
| Date | 2026-10-17 |
| Operator | guru-bharadwaj20 |
| Cluster | kubeadm v1.35.9, Cilium 1.18.2, 3 nodes (cp-1, worker-1, worker-2) |
| Namespace | `satark-dev` (Pod Security `restricted`) |
| Image tag | `lab`, built locally and imported with `scripts/load-lab-images.sh` |
| Data layer | CloudNativePG 1.24.1 (chart 0.22.1), `satark-pg` 3 instances on PostgreSQL 16.4; Valkey 8.0.1 |
| Gateway | Envoy Gateway v1.2.1, Gateway API CRDs v1.2.1, GatewayClass `satark` accepted |

## Steps

Run from Git Bash on the Windows host with
`KUBECONFIG=~/.kube/satark-onprem.conf`. WSL2 cannot reach the host-only
network.

```
scripts/platform/install-cnpg.sh
scripts/platform/create-pg-secret.sh
scripts/platform/run-migrations.sh
scripts/platform/install-valkey.sh
scripts/platform/install-envoy-gateway.sh
IMAGE_TAG=lab scripts/load-lab-images.sh
# Telegram bot token stored once as Secret satark-dev/telegram-bot
IMAGE_TAG=lab NAMESPACE=satark-dev scripts/deploy-mvp.sh
```

## Result

```
deployment.apps/gateway-api     2/2     2            2           71s
deployment.apps/qr-decoder      2/2     2            2           80s
deployment.apps/telegram-bot    1/1     1            1           62s
deployment.apps/text-analyzer   2/2     2            2           89s

PASS  deployment text-analyzer available
PASS  deployment qr-decoder available
PASS  deployment gateway-api available
PASS  deployment telegram-bot available
PASS  GET /healthz -> 200
PASS  GET /readyz -> 200
PASS  in-cluster scan flags the scam sample
PASS  in-cluster rate-limit headers
Smoke tests passed
```

Extra checks:

| Check | Result |
|---|---|
| Smoke-test scan stored in `scans`, input only as a 32-byte HMAC | ☑ (`text`, `suspicious`, score 56, 4 reason codes) |
| gateway-api logs `scan persistence enabled` and `rate limiting enabled` (1 rps, burst 30) | ☑ |
| telegram-bot authenticates and long-polls | ☑ (`bot authenticated`, `polling for updates`) |
| Migration job `1/u init` | ☑ |
| HTTPRoutes created for `api.satark.example` and `bot.satark.example` | ☑ |

The public Gateway `satark-public` was not applied. It terminates TLS with a
cert-manager certificate, and cert-manager comes later. The smoke test ran
in-cluster through a port-forward, without `GATEWAY_URL`.

## Issues found and fixes

- gateway-api never connected its store, Valkey or rate-limit packages:
  `main.go` only built the analyzer fan-out. It now connects to PostgreSQL
  when `DATABASE_URL` is set, saving every successful scan through
  `scan.Recorder`, and connects to Valkey when `VALKEY_ADDR` is set, rate
  limiting `/v1` (`RATE_LIMIT_RPS`, `RATE_LIMIT_BURST`). With neither set,
  Docker Compose runs it as before. The verdict cache is still not connected.
- The migration job's `pg_isready` wait never ended ("no attempt"): uid 65532
  has no passwd entry, so it had no default user. It now passes `-U satark`.
- The EnvoyProxy service asked MetalLB for a pool named `default`; the lab
  pool is `lab-pool`.
- The `build` workflow fails before building anything, because
  `aquasecurity/trivy-action@0.28.0` no longer resolves. So no images are on
  GHCR. The lab uses locally built images until CI is fixed.
- Windows Vagrant refuses VMs created by WSL Vagrant (UID check), so
  `load-lab-images.sh` uses the forwarded SSH ports and per-machine keys
  directly.
