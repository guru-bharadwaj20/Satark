# GD-08: WireGuard tunnel down

| | |
| --- | --- |
| Status | **Planned (requires manual execution against AWS and the live cluster)** |
| Script | `tests/chaos/gd-08-wireguard-down.sh` |
| Runbook under test | RB-010 (tunnel) |
| Expected duration | 30 minutes |

## Hypothesis

Public traffic enters at the AWS edge EC2 (TCP proxy) and reaches on-prem
over WireGuard. With the tunnel down:

- The edge proxy still accepts TCP connections, so DNS and TLS handshakes
  to the edge look fine, but connections to the backend time out. Users see
  timeouts, not a fast error.
- Everything inside the cluster is healthy; in-cluster alerts stay quiet.
- Detection comes from the external synthetic probe (CloudWatch Synthetics
  or Route 53 health check) and the `satark_http_requests_total` rate
  dropping to near zero.
- The Telegram bot keeps working for outbound calls (it polls/pushes from
  on-prem over the normal internet path), but webhook delivery fails.

## Blast radius and abort criteria

- Scope: all public traffic. Keep the window short (target 15 minutes).
- Abort action: `... revert`.

## Procedure

1. `... observe` (recent handshake, `active`).
2. `... inject`.
3. Responder starts from the external probe alert. Expected path: in-cluster
   healthy -> traffic rate dropped -> edge reachable? -> `wg show` on edge and
   on the on-prem peer (no recent handshake) -> RB-010.
4. `... revert`, confirm handshake within 25 s (PersistentKeepalive).

## Things to check

- Is there an alert on WireGuard handshake age (e.g. node-exporter textfile
  `satark_wireguard_last_handshake_seconds`)?
- Should the edge proxy fail fast (serve a static "temporarily unavailable"
  page) when the backend is unreachable?
- Does the tunnel come back on its own after an edge reboot?

## Results

| Measure | Value |
| --- | --- |
| Time to alert | |
| Time to diagnose | |
| Probe failures | |
| Handshake after revert | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
