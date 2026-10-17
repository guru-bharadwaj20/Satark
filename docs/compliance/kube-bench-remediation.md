# kube-bench remediation log

Tracks how high-severity CIS Kubernetes Benchmark findings are fixed
(task 9.10). Findings are produced by `scripts/kube-bench/run.sh`, filed as
issues by `scripts/kube-bench/file_issues.py`, and fixed in code by
`infra/ansible/playbooks/cis-hardening.yml`.

Benchmark: CIS Kubernetes v1.12 (`--benchmark cis-1.12`), kube-bench v0.16.0,
on kubeadm v1.35.9 with Cilium kube-proxy replacement.

## Process

1. Run the benchmark: `scripts/kube-bench/run.sh`. It runs one Job on the
   control plane (all targets) and one on a worker (`--targets node`).
2. Preview issues: `scripts/kube-bench/file_issues.py docs/compliance/kube-bench/<stamp>-*.json`, then re-run with `--apply`.
3. Apply fixes from `infra/ansible`: `ansible-playbook playbooks/cis-hardening.yml --check --diff`, then without `--check`.
4. Re-run the benchmark and close each issue whose check now shows PASS.
5. Anything not fixed gets an entry in the exceptions table below, and its
   issue is closed as not planned with a link here.

`playbooks/upgrade.yml` imports `cis-hardening.yml` at the end, because
`kubeadm upgrade` rewrites the static pod manifests and package upgrades reset
file modes. New clusters get the same API server, controller-manager and
kubelet settings from `roles/kubeadm_cp/templates/kubeadm-config.yaml.j2`.

## Findings from the first run

| CIS ID | Issue | Finding | Outcome |
|---|---|---|---|
| 1.1.12 | #1 | etcd data dir not owned by `etcd:etcd` | Fixed |
| 1.2.5 | #2 | `--kubelet-certificate-authority` not set | Fixed |
| 1.2.30 | #3 | `--service-account-extend-token-expiration` not `false` | Fixed |
| 1.3.7 | #4 | Controller-manager bound to all interfaces | Accepted exception |
| 1.4.2 | #5 | Scheduler bound to all interfaces | Accepted exception |
| 4.1.1 | #6 | Kubelet service file mode `644` | Fixed |
| 4.1.9 | #7 | Kubelet `config.yaml` mode `644` | Fixed |
| 4.3.1 | #8 | kube-proxy metrics not bound to localhost | Not applicable |

## Fixes in the hardening playbook

| CIS ID | Finding | Fix |
|---|---|---|
| 1.1.1-1.1.8, 1.1.13-1.1.18 | Control-plane manifests and kubeconfigs | `0600 root:root` (only files that exist; `super-admin.conf` is on the first control-plane node only) |
| 1.1.11/1.1.12 | etcd data dir permissions and ownership | `etcd` system user, `/var/lib/etcd` `0700 etcd:etcd` |
| 1.1.19-1.1.21 | PKI ownership and permissions | `root:root`, keys and certs `0600` |
| 1.2.5 | Kubelet serving certs not verified | `--kubelet-certificate-authority=/etc/kubernetes/pki/ca.crt` |
| 1.2.14 | NodeRestriction admission | `--enable-admission-plugins=NodeRestriction` |
| 1.2.15, 1.3.2, 1.4.1 | Profiling enabled | `--profiling=false` on API server, controller-manager, scheduler |
| 1.2.16-1.2.19 | No audit logging | Audit policy and log flags, see task 9.14 |
| 1.2.21, 1.2.22, 1.2.29 | Request timeout, SA lookup, TLS floor | `--request-timeout=60s`, `--service-account-lookup=true`, `--tls-min-version=VersionTLS12` |
| 1.2.30 | Service account tokens extended past their expiry | `--service-account-extend-token-expiration=false` |
| 1.3.1 | Terminated pods never garbage collected | `--terminated-pod-gc-threshold=1000` |
| 4.1.1, 4.1.5, 4.1.9 | Kubelet unit, drop-in, kubeconfig and config modes | `0600 root:root` |
| 4.1.7/4.1.8 | Client CA file | `0600 root:root` |
| 4.2.4, 4.2.6, 4.2.11-4.2.13 | Kubelet read-only port, kernel defaults, cert rotation, ciphers | Merged into `/var/lib/kubelet/config.yaml` as YAML; sysctls in `/etc/sysctl.d/90-kubelet.conf` |

Notes from applying it:

- **4.1.1 checks the unit file, not the drop-in.** kube-bench audits
  `/lib/systemd/system/kubelet.service`. The playbook sets both the unit and
  the `10-kubeadm.conf` drop-in (under `/usr/lib`, which `/lib` links to).
- **1.1.12 needs the host's user database.** The ownership was right after the
  first apply, but the kube-bench container has no `etcd` user, so `stat`
  printed `UNKNOWN:UNKNOWN`. The control-plane Job now mounts the host's
  `/etc/passwd` and `/etc/group` read-only.
- **The kubelet config is merged, not appended.** Appending text duplicates
  keys kubeadm already writes. The playbook reads the YAML, combines the CIS
  settings, and only writes and restarts kubelet when something changed.
- **The API server restart is waited on.** The playbook checks the running
  process's flags before checking `/readyz`, so it does not move to the next
  node while the old API server is still answering.

## Kubelet serving certificate renewal

With `--kubelet-certificate-authority` set, the API server rejects a kubelet
whose serving certificate is not signed by the cluster CA, so `kubectl logs`,
`exec` and `port-forward` to that node fail. The serving certificates come
from `serverTLSBootstrap` CSRs, which Kubernetes does not approve on its own.

- The `kubeadm_cp` and `kubeadm_worker` roles approve the CSRs at join time.
- `cis-hardening.yml` approves any pending `kubernetes.io/kubelet-serving`
  CSR whose requester is `system:node:*` on every run.
- kubelets renew at 70-90% of the one-year lifetime, so a renewal can arrive
  between runs. Follow-up: deploy
  [kubelet-csr-approver](https://github.com/postfinance/kubelet-csr-approver)
  to approve them automatically, checking the node name and IPs.
- If logs or exec break on one node, check
  `kubectl get csr | grep Pending` first.

## Accepted exceptions

| CIS ID | Reason | Compensating control | Review date |
|---|---|---|---|
| 1.2.1 (anonymous auth) | kubeadm's liveness, readiness and startup probes call `/livez` and `/readyz` without credentials; `--anonymous-auth=false` makes the API server fail its own probes | RBAC grants `system:anonymous` only the health and version endpoints (`system:public-info-viewer`) | 2027-01-17 |
| 1.2.13 (AlwaysPullImages) | Breaks offline lab nodes and slows rollouts | Kyverno digest pinning (`verify-image-signatures`) | 2027-01-17 |
| 1.3.7, 1.4.2 (bind address) | kube-prometheus-stack scrapes the controller-manager and scheduler on the node IP; binding to `127.0.0.1` stops the scrape and fires `KubeControllerManagerDown` / `KubeSchedulerDown` | Secure ports only (10257, 10259): TLS plus authentication and authorization, profiling off. Set `cis_bind_localhost: true` where nothing scrapes them | 2027-01-17 |
| 4.3.1 (kube-proxy metrics) | Not applicable: Cilium replaces kube-proxy, so there is no kube-proxy to bind | Cilium and Hubble metrics are scraped by Prometheus | n/a |
| 5.x (manual policies) | Covered by workload policy instead of kube-bench | PSA `restricted` + Kyverno policies | 2027-01-17 |

## Results

| Date | Node | PASS | FAIL | WARN | Report |
|---|---|---|---|---|---|
| 2026-10-17 (before) | cp-1 | 68 | 8 | 55 | `kube-bench/kube-bench-20261017T090313Z-control-plane.json` |
| 2026-10-17 (before) | worker-1 | 16 | 3 | 6 | `kube-bench/kube-bench-20261017T090313Z-worker.json` |
| 2026-10-17 (after) | cp-1 | 75 | 3 | 53 | `kube-bench/kube-bench-20261017T131416Z-control-plane.json` |
| 2026-10-17 (after) | worker-1 | 19 | 1 | 5 | `kube-bench/kube-bench-20261017T131416Z-worker.json` |

The remaining FAILs after hardening are 1.3.7, 1.4.2 and 4.3.1 on the control
plane and 4.3.1 on the worker, all listed under accepted exceptions above.
Nodes, `kubectl logs` and `kubectl exec` were checked after the API server
restarted with `--kubelet-certificate-authority`.
