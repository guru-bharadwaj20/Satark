# GD-NN: Title

| | |
| --- | --- |
| Status | Executed on YYYY-MM-DD |
| Facilitator | |
| Responder(s) | |
| Script | `tests/chaos/gd-NN-*.sh` |
| Expected duration | |

## Hypothesis

What we believe will happen, in terms users would notice.

## Blast radius and abort criteria

- Scope:
- Abort if:
- Abort action: `<script> revert`

## Pre-checks

- [ ] `kubectl satark doctor` is clean (or known warnings noted)
- [ ] Latest Velero and CNPG backups are recent
- [ ] Status channel notified

## Procedure

1. `<script> observe` (baseline)
2. `<script> inject`
3. Responder diagnoses using only alerts, dashboards, runbooks (and opsbot if
   this is an opsbot run). Facilitator records times.
4. `<script> revert` (or the responder's fix)
5. `<script> observe`

## Results

| Measure | Value |
| --- | --- |
| Time to detect (alert fired) | |
| Time to diagnose (correct root cause stated) | |
| Time to mitigate | |
| User-facing errors (probe failures) | |
| Hypothesis held? | yes / no |

## Timeline

Paste from `docs/game-days/logs/<name>-<date>.log`.

## What we learned

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
