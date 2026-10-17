# Phase 1 cluster verification report

Result of running `scripts/bootstrap.sh` and `scripts/verify-cluster.sh` on a
freshly built lab.

| Field | Value |
|---|---|
| Date | 2026-10-17 |
| Operator | guru-bharadwaj20 |
| Host (CPU / RAM / OS) | 32 cores / 128 GB / Windows 11 Pro + WSL2 (Ubuntu 24.04), VirtualBox 7.2.20, Vagrant 2.4.9 |
| Kubernetes version (`kubectl version`) | v1.35.9 |
| Cilium version (`cilium version` or chart) | 1.18.2 |
| Nodes | cp-1, worker-1, worker-2 |
| Bootstrap duration (`time ./scripts/bootstrap.sh`) | ~20 min for the final clean playbook run (VM creation and first-boot apt upgrades not included) |

## Checks

| Check | Command | Result |
|---|---|---|
| All nodes Ready | `kubectl get nodes` | ☑ |
| System pods Running | `kubectl get pods -A` | ☑ |
| Cluster DNS resolves `kubernetes.default` | `verify-cluster.sh` | ☑ |
| External DNS resolves through CoreDNS | `verify-cluster.sh` | ☑ |
| Test LoadBalancer gets an IP from `192.168.56.240-250` | `verify-cluster.sh` | ☑ (192.168.56.240) |
| LoadBalancer IP answers from the host | `curl http://<lb-ip>/` | ☑ |
| `kubectl top nodes` works | `verify-cluster.sh` | ☑ (all 3 nodes) |
| `local-path` is the default StorageClass | `kubectl get sc` | ☑ |
| Hubble sees flows | `kubectl -n kube-system exec ds/cilium -- hubble observe --last 5` | ☑ |
| API audit log is written | `sudo tail -n1 /var/log/kubernetes/audit/audit.log` on cp-1 | ☑ |
| Re-running `site.yml` reports `changed=0` for most tasks | `ansible-playbook playbooks/site.yml` | ☑ (cp-1 changed=2, workers changed=1, failed=0) |

`verify-cluster.sh` was run from Git Bash on the Windows host: WSL2 cannot
reach the VirtualBox host-only network, the Windows host can.

## Output

```
== Nodes
NAME       STATUS   ROLES           AGE    VERSION   INTERNAL-IP     EXTERNAL-IP   OS-IMAGE             KERNEL-VERSION     CONTAINER-RUNTIME
cp-1       Ready    control-plane   24m    v1.35.9   192.168.56.10   <none>        Ubuntu 24.04.3 LTS   6.8.0-86-generic   containerd://2.2.1
worker-1   Ready    worker          4m8s   v1.35.9   192.168.56.11   <none>        Ubuntu 24.04.3 LTS   6.8.0-86-generic   containerd://2.2.1
worker-2   Ready    worker          4m1s   v1.35.9   192.168.56.12   <none>        Ubuntu 24.04.3 LTS   6.8.0-86-generic   containerd://2.2.1
  [PASS] all 3 nodes Ready
== System pods
  [PASS] all system pods Running
== In-cluster DNS
  [PASS] kubernetes.default.svc.cluster.local resolves
  [PASS] external name resolves through CoreDNS
== LoadBalancer
  [PASS] LoadBalancer got 192.168.56.240
  [PASS] LoadBalancer 192.168.56.240 answers HTTP
== Metrics and storage
  [PASS] metrics-server serves node metrics
  [PASS] default StorageClass is local-path

Cluster verification PASSED
```

## Issues found and fixes

- Scripts checked out with CRLF line endings on Windows and failed in WSL
  (`bash\r`). Added `.gitattributes` forcing LF for shell, Python, YAML and
  Ansible files.
- `setup-control-node.sh` did not install a Linux Vagrant. It now installs
  the HashiCorp package pinned to the Windows version (2.4.9).
- Vagrant under WSL2 could not reach `127.0.0.1:2222`. Fixed by enabling
  `networkingMode=mirrored` in `%USERPROFILE%\.wslconfig`.
- The host-only NIC (`eth1`) came up without an address on bento/ubuntu-24.04.
  The Vagrant provisioner now writes its own netplan file for it, and the boot
  timeout was raised to 15 minutes.
- WSL2 cannot reach 192.168.56.0/24 even in mirrored mode. VMs now have fixed
  forwarded SSH ports and `bootstrap.sh` adds
  `inventory/onprem/wsl-ports.yml` under WSL. `node_ip` is set per host so
  the cluster still uses the host-only addresses.
- bento masks `apt-daily*.service`, so the apt timers would not start. The
  common role unmasks them with `systemctl unmask`.
- First-boot unattended-upgrades held the dpkg lock. OS plays set
  `lock_timeout: 600` for `ansible.builtin.apt`.
- The local kubeconfig chmod tried to `sudo` on the control node: the
  `ansible_become` inventory variable outranks the `become` keyword, so the
  task now overrides the variable.
- fail2ban (aggressive mode) banned the VirtualBox NAT gateway `10.0.2.2`,
  which is where all forwarded SSH arrives from. Added it to `ignoreip`.
- Cilium and MetalLB were configured for `enp0s8`; bento names the host-only
  NIC `eth1`. Both now use the `lab_interface` inventory variable.
- worker-1's kubelet serving CSR was created after the approval task had
  already run, leaving `kubectl exec`/metrics broken on that node. The task now
  retries until an approved serving CSR exists for the node.
