# GD-04: Apply a bad NetworkPolicy

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
| Script | `tests/chaos/gd-04-bad-networkpolicy.sh` |
| Expected duration | 30 minutes |

## Hypothesis

A policy meant to restrict `url-intel` to traffic from `gateway-api` has a
misspelt selector and no DNS egress rule. After it is applied:

- `gateway-api` calls to `url-intel` time out (dropped, not refused), so link
  checks return a degraded verdict after the client timeout; text checks keep
  working.
- `url-intel` pods stay Running and Ready (probes come from the kubelet on the
  node, which Cilium allows by default), so `kubectl get pods` looks healthy.
- Hubble shows `DROPPED` flows with `policy-verdict:none` to `url-intel`.
- Detection relies on the `url-intel` error-rate / latency alert, not on pod
  health.

## Blast radius and abort criteria

- Scope: link checks only. Abort after 20 minutes without diagnosis.
- Abort action: `... revert`.

## Procedure

1. `... observe` (no gd04 policy, no drops).
2. `... inject`.
3. Responder starts from the latency alert. Expected path: gateway logs show
   `context deadline exceeded` to url-intel -> pods healthy -> suspect network ->
   `hubble observe --verdict DROPPED` -> find the policy -> check selector labels
   (`kubectl get pods -n satark --show-labels`).
4. `... revert`.

## Prevention to evaluate

- Kyverno or CI test that every NetworkPolicy `podSelector`/`from` selector
  matches at least one existing pod.
- A default "allow DNS egress" policy so no app policy can forget it.
- Argo CD drift detection: was the out-of-band policy visible?

## Results

| Measure | Value |
| --- | --- |
| Time to alert | |
| Time to diagnose | |
| Failed link checks | |
| Was Hubble used? | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
