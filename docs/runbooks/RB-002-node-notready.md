# RB-002: Node NotReady

| | |
|---|---|
| **Alerts** | `SatarkNodeNotReady`, `SatarkNodePressure`, `SatarkNodeFilesystemAlmostFull` |
| **Severity** | Critical (NotReady), warning (pressure, disk) |
| **Impact** | Pods on the node stop being scheduled; after ~5 minutes they are evicted and rescheduled elsewhere if capacity allows. On a small cluster, losing one worker can leave services under-replicated. |
| **Dashboards** | Satark / Cluster health, Satark / Node and Linux |

## 1. Triage

```bash
kubectl get nodes -o wide
NODE=<node from the alert>
kubectl describe node $NODE | sed -n '/Conditions:/,/Addresses:/p'
kubectl get pods -A -o wide --field-selector spec.nodeName=$NODE | grep -v Running
```

Look at the `Ready` condition's `Reason` and `Message`:

| Reason / message | Meaning | Go to |
|---|---|---|
| `NodeStatusUnknown`, "Kubelet stopped posting node status" | Node is down, unreachable, or kubelet is dead | 2 |
| `KubeletNotReady`, "container runtime is down" | containerd problem | 3 |
| `KubeletNotReady`, "network plugin not ready" / "cni" | Cilium agent down on the node | 4 |
| `MemoryPressure` / `DiskPressure` / `PIDPressure` true | Resource exhaustion | 5 |

Is the control plane healthy? If **every** node is NotReady at once, the
problem is the API server or etcd, not the nodes: check
`kubectl -n kube-system get pods -l tier=control-plane` from a control-plane node.

## 2. Node unreachable or kubelet down

```bash
ping -c3 <node IP>
ssh <node> 'systemctl status kubelet --no-pager; journalctl -u kubelet --since "15 min ago" --no-pager | tail -50'
```

- **No ping / no SSH:** the VM or host is down. On-prem: check the hypervisor /
  Vagrant (`vagrant status`), power and network. Cloud: check the instance in
  the AWS console. Start it again.
- **kubelet failing:** common causes are an expired kubelet client certificate
  (`x509: certificate has expired` in the journal), swap re-enabled after a
  reboot, or a full disk.

```bash
ssh <node> 'sudo swapoff -a; sudo systemctl restart kubelet'
# expired certificates on kubeadm nodes:
ssh <node> 'sudo kubeadm certs check-expiration'
```

## 3. Container runtime

```bash
ssh <node> 'sudo systemctl status containerd --no-pager; sudo crictl info | head -20'
ssh <node> 'sudo systemctl restart containerd && sudo systemctl restart kubelet'
```

## 4. CNI (Cilium) not ready

```bash
kubectl -n kube-system get pods -l k8s-app=cilium -o wide | grep $NODE
kubectl -n kube-system logs <cilium-pod> --tail=100
cilium status --wait   # from a machine with the cilium CLI
```

Delete the Cilium pod on that node to restart it. If it crash-loops, follow
RB-001 for that pod.

## 5. Resource pressure

```bash
ssh <node> 'df -h /var/lib/containerd /var/lib/kubelet /; free -m'
ssh <node> 'sudo crictl rmi --prune'                    # unused images
ssh <node> 'sudo journalctl --vacuum-size=500M'         # oversized journal
```

For memory pressure, find the heaviest pods in the Node and Linux dashboard
and check for a leak or missing limit (RB-001, section 2).

## 6. Drain while you fix

If the fix takes more than a few minutes, or the node needs a reboot:

```bash
kubectl cordon $NODE
kubectl drain $NODE --ignore-daemonsets --delete-emptydir-data --timeout=5m
# ... fix / reboot ...
kubectl uncordon $NODE
```

PodDisruptionBudgets may block the drain for single-replica workloads. Do not
use `--disable-eviction` on a node running Postgres (`satark-pg`) without
first checking that a CloudNativePG replica is healthy:
`kubectl -n satark-data get cluster satark-pg`.

## 7. Verify

- `kubectl get nodes` shows `Ready` for 10 minutes.
- No `Pending` pods: `kubectl get pods -A --field-selector status.phase=Pending`.
- Alerts resolved in the Telegram ops channel.

## 8. Follow-up

If the node was lost for hardware or capacity reasons, record it and consider
adding capacity. If a certificate expired, check the renewal automation.
