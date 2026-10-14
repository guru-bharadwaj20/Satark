# Phase 1 cluster verification report

Fill this in after running `scripts/bootstrap.sh` and `scripts/verify-cluster.sh`
on a freshly built lab, and attach the script output.

| Field | Value |
|---|---|
| Date | _YYYY-MM-DD_ |
| Operator | _name_ |
| Host (CPU / RAM / OS) | _e.g. 8 cores / 16 GB / Windows 11 + WSL2_ |
| Kubernetes version (`kubectl version`) | _v1.35.x_ |
| Cilium version (`cilium version` or chart) | _1.18.x_ |
| Nodes | cp-1, worker-1, worker-2 |
| Bootstrap duration (`time ./scripts/bootstrap.sh`) | _mm:ss_ |

## Checks

| Check | Command | Result |
|---|---|---|
| All nodes Ready | `kubectl get nodes` | ☐ |
| System pods Running | `kubectl get pods -A` | ☐ |
| Cluster DNS resolves `kubernetes.default` | `verify-cluster.sh` | ☐ |
| External DNS resolves through CoreDNS | `verify-cluster.sh` | ☐ |
| Test LoadBalancer gets an IP from `192.168.56.240-250` | `verify-cluster.sh` | ☐ |
| LoadBalancer IP answers from the host | `curl http://<lb-ip>/` | ☐ |
| `kubectl top nodes` works | `verify-cluster.sh` | ☐ |
| `local-path` is the default StorageClass | `kubectl get sc` | ☐ |
| Hubble sees flows | `kubectl -n kube-system exec ds/cilium -- hubble observe --last 5` | ☐ |
| API audit log is written | `sudo tail -n1 /var/log/kubernetes/audit/audit.log` on cp-1 | ☐ |
| Re-running `site.yml` reports `changed=0` for most tasks | `ansible-playbook playbooks/site.yml` | ☐ |

## Output

```
<paste scripts/verify-cluster.sh output here>
```

## Issues found and fixes

- _none_
