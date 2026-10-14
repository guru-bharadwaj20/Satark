# RB-NNN: Short scenario title

- **Alerts:** `AlertName` (severity: critical | warning)
- **Owner:** @handle
- **Last reviewed:** YYYY-MM-DD
- **Related:** ADR-NNN, SOP-NN, dashboards

## Symptoms

What the on-call engineer sees: alert text, dashboard panels, user reports.

## Impact

Who and what is affected (e.g. Telegram users get no verdict), and which SLO is at risk.

## Diagnosis

Run these in order. Each step says what a good and a bad result look like.

```bash
kubectl -n satark get pods
kubectl -n satark describe pod <pod>
kubectl -n satark logs <pod> --previous --tail=100
```

## Fix

Numbered steps per likely cause. Prefer safe, reversible actions first.

1. **Cause A:** steps.
2. **Cause B:** steps.

## Verify

How to confirm recovery: the alert has resolved, the metric is back to normal, and a test check works end to end.

```bash
kubectl -n satark get pods
```

## Escalation

When to escalate (e.g. no recovery after 30 minutes, or data loss suspected) and to whom.

## Prevention

Follow-ups that stop this happening again: alerts, limits, tests, ADRs. Link the issues.
