# Upgrade under load: N-1 to N

Status: **template, not yet executed.** Fill in after running
`tests/load/upgrade-under-load.sh <target-minor> <inventory>`.

## Setup

| Field | Value |
| --- | --- |
| Date (UTC) | |
| From version | v1.__.__ |
| To version | v1.__.__ |
| Nodes | 1 control plane, 2 workers |
| Load | k6 `upgrade-soak.js`, constant 20 req/s on `POST /v1/check/text` |

## Results

Copy from `tests/load/results/<run>/summary.txt`.

| Metric | Value | Target |
| --- | --- | --- |
| Upgrade duration | | < 60 min |
| Requests sent | | |
| Failed requests (`satark_upgrade_errors`) | | 0 ideal, < 0.1% |
| p95 latency | | < 800 ms |

## Timeline

| Time | Event | Errors in window |
| --- | --- | --- |
| | Control plane drained | |
| | API server restarted | |
| | worker-1 drained | |
| | worker-2 drained | |

## Findings

- Where did errors come from (API server restart, a single-replica Deployment,
  missing PDB, slow readiness probe)?
- Fixes, each with an owner and a link to the commit.
