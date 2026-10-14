# kube-bench remediation log

Tracks how high-severity CIS Kubernetes Benchmark findings are fixed
(task 9.10). Findings are produced by `scripts/kube-bench/run.sh`, filed as
issues by `scripts/kube-bench/file_issues.py`, and fixed in code by
`infra/ansible/playbooks/cis-hardening.yml`.

## Process

1. Run the benchmark: `scripts/kube-bench/run.sh`.
2. Preview issues: `scripts/kube-bench/file_issues.py docs/compliance/kube-bench/<report>.json`, then re-run with `--apply`.
3. Apply fixes: `ansible-playbook ... cis-hardening.yml --check --diff`, then without `--check`.
4. Re-run the benchmark and close each issue whose check now shows PASS.
5. Anything not fixed gets an entry in the exceptions table below.

## Fixes in the hardening playbook

| CIS ID | Finding | Fix |
|---|---|---|
| 1.1.1-1.1.8 | Control-plane manifest permissions too open | `0600 root:root` |
| 1.1.11/1.1.12 | etcd data dir permissions/ownership | `0700 etcd:etcd` |
| 1.1.19-1.1.21 | PKI files and directory ownership/permissions | `root:root`, keys/certs `0600` |
| 1.2.1 | API server anonymous auth enabled | `--anonymous-auth=false` |
| 1.2.5 | Kubelet certificate authority not set | `--kubelet-certificate-authority` |
| 1.2.15 | API server profiling enabled | `--profiling=false` |
| 1.2.16-1.2.19 | No audit logging | Audit policy and log flags, see task 9.14 |
| 1.3.2/1.4.1 | Controller-manager/scheduler profiling | `--profiling=false` |
| 1.3.7/1.4.2 | Controller-manager/scheduler bound to all interfaces | `--bind-address=127.0.0.1` |
| 4.1.x | Kubelet config permissions | `0600 root:root` |
| 4.2.1/4.2.6 | Kubelet read-only port, kernel defaults | `readOnlyPort: 0`, `protectKernelDefaults: true` |

## Accepted exceptions

| CIS ID | Reason | Compensating control | Review date |
|---|---|---|---|
| 1.2.13 (AlwaysPullImages) | Breaks offline lab nodes and slows rollouts | Kyverno digest pinning (`verify-image-signatures`) | next quarter |
| 5.x (manual policies) | Covered by workload policy instead of kube-bench | PSA `restricted` + Kyverno policies | next quarter |

## Results

| Date | PASS | FAIL | WARN | Report |
|---|---|---|---|---|
| _pending first run on the live cluster_ | | | | |
