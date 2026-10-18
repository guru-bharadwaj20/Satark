# GD-04: Apply a bad NetworkPolicy

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
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

The lab has no `url-intel`, so the policy was pointed at `text-analyzer`
(`NS=satark-dev TARGET=text-analyzer PORT=8000`). That puts the text-scan
path in the blast radius instead of link checks. The probe sent
`POST /v1/scan/text` through the gateway once a second, with a 5 s client
timeout so the gateway's own answer was visible.

Two variants:

- **add** (the script as written): a second policy next to the chart's own
  `text-analyzer` policy.
- **replace** (new `MODE=replace`): the bad spec written over the chart's
  policy, as a bad `networkPolicy` values change would do through Helm or
  Argo CD.

| Measure | add | replace |
| --- | --- | --- |
| Time to alert | n/a (no Prometheus in the lab) | n/a |
| Time to diagnose | nothing to diagnose | under 1 min: gateway log `context deadline exceeded` to text-analyzer → pods Ready → Hubble |
| Failed text checks | 0 / 300 | 300 / 300, each a `503` after ~2 s (the gateway's `ANALYZER_TIMEOUT`) |
| Pods Ready during the fault | yes | yes (kubelet probes are not affected by the policy) |
| Was Hubble used? | yes, no drops | yes: `policy-verdict:none INGRESS DENIED`, `Policy denied DROPPED (TCP Flags: SYN)` from both gateway pods |
| Recovery after revert | n/a | first request after the revert returned 200 |

Raw logs: `logs/gd-04-bad-networkpolicy-20261018.log` and the per-second
trace `logs/gd-04-bad-networkpolicy-replace-trace-20261018.log`.

Caveat: the Windows host was running heavy WSL jobs alongside the lab VMs.

## Timeline (UTC)

| Time | Event |
| --- | --- |
| 14:33:41 | add: `gd04-url-intel-lockdown` applied next to the chart policy |
| 14:33:41 → 14:39:11 | 300 / 300 probes succeed |
| 14:39:23 | replace: chart policy `text-analyzer` overwritten with the bad spec |
| 14:39:24 → 14:54:51 | every text scan returns 503 after ~2 s; Hubble shows SYNs dropped at `text-analyzer:8000` |
| 14:54:52 | Revert: chart policy restored; next request 200 |

## What we learned

- **The scenario as written could not happen here.** NetworkPolicies add
  up: traffic is allowed if any policy that selects the pod allows it.
  Every satark chart already ships its own default-deny-plus-allow policy
  (`satark-common.networkpolicy`, on by default). A second, wrong policy
  therefore cannot cut anything off while that one allows the traffic. The
  "add" run proved it: 0 failures with the bad policy in place.
- The real risk is a bad change to the chart's own policy, such as a
  misspelt `ingressFrom` label or a dropped DNS rule in values. That
  behaved exactly as hypothesised: traffic dropped, not refused; clients
  waited the full 2 s analyzer timeout; pods stayed Running and Ready, so
  `kubectl get pods` looked healthy. The gateway log and Hubble pointed
  straight at it.
- `hubble observe` needs `kubectl -n kube-system port-forward
  svc/hubble-relay 4245:80` first; without it the script's observe step
  prints a connection error.
- Argo CD drift was not tested: `text-analyzer` is a plain Helm release in
  the lab. Through Argo CD, a hand-applied change to a tracked policy would
  show OutOfSync, and self-heal would revert it within seconds.

## Prevention evaluated

- **CI selector check:** added. `scripts/check-networkpolicy-selectors.py`
  runs in the `helm` workflow over every chart rendered with each
  environment's values. It fails if a policy's `podSelector`, or a
  same-namespace `from`/`to` pod selector, matches no workload. It also
  fails if a pod with restricted egress has no rule allowing DNS across all
  the policies that select it. The workflow self-tests it against this game
  day's policy, which it rejects for both flaws.
- **Default allow-DNS policy:** not needed for the charts: the shared
  template always allows kube-dns, and the check above catches a policy
  that drops it.
- **Kyverno:** an admission-time check would also catch hand-applied
  policies. Left as an option; CI covers the Git path.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| A wrong selector or missing DNS rule in a chart policy breaks the service while pods look healthy | CI check `check-networkpolicy-selectors.py` in the `helm` workflow | guru-bharadwaj20 | ✅ |
| The script only tested the additive case, which cannot fail | `MODE=replace` overwrites the chart policy and restores it on revert | guru-bharadwaj20 | ✅ |
| Hubble needs a port-forward the script does not mention | Noted above; RB-005 already uses `hubble observe` the same way | guru-bharadwaj20 | ✅ |
| Alert timing untested | Re-run on the cloud cluster with Prometheus (`SatarkHighErrorRate`) | guru-bharadwaj20 | |
