# RB-005: In-cluster DNS failures

| | |
|---|---|
| **Alerts** | `SatarkCoreDNSDown`, `SatarkDNSServfailHigh`, `SatarkDNSLatencyHigh` |
| **Severity** | Critical (CoreDNS down), warning (SERVFAIL, latency) |
| **Impact** | Services cannot reach Postgres, Valkey, NATS or each other by name. `url-intel` DNS checks (A/AAAA/NS/MX) return errors, so URL verdicts degrade to `unknown`. Outbound calls (LLM, Safe Browsing, Telegram) fail. |
| **Dashboards** | Satark / Cluster health (CoreDNS responses), Hubble UI (`kube-system`) |

## 1. Triage: is it cluster DNS or upstream DNS?

```bash
kubectl -n kube-system get pods -l k8s-app=kube-dns -o wide
kubectl -n kube-system get svc kube-dns
kubectl -n kube-system get endpointslices -l kubernetes.io/service-name=kube-dns

# Throwaway debug pod in the app namespace
kubectl -n satark run dnsdebug --rm -it --restart=Never \
  --image=registry.k8s.io/e2e-test-images/agnhost:2.47 -- sh
# inside the pod:
nslookup kubernetes.default.svc.cluster.local     # cluster name
nslookup valkey.satark-data.svc.cluster.local     # cross-namespace
nslookup ghcr.io                                   # upstream
cat /etc/resolv.conf
```

| Cluster names | External names | Likely cause | Go to |
|---|---|---|---|
| fail | fail | CoreDNS down or unreachable | 2 |
| work | fail | Upstream resolvers / node egress | 3 |
| fail from some pods only | — | NetworkPolicy or a node's Cilium agent | 4 |
| work but slow | slow | CoreDNS overloaded or `ndots` amplification | 5 |

## 2. CoreDNS down or unreachable

```bash
kubectl -n kube-system logs -l k8s-app=kube-dns --tail=50
kubectl -n kube-system describe deploy coredns
kubectl -n kube-system get cm coredns -o yaml
```

- Pods crash-looping: follow RB-001. A recent edit to the `coredns` ConfigMap
  with a syntax error is the usual cause; logs show `plugin/...: Error`.
  Revert the ConfigMap change and `kubectl -n kube-system rollout restart deploy/coredns`.
- `loop` plugin detected a forwarding loop (`Loop ... detected`): the node's
  `/etc/resolv.conf` points at a local stub (127.0.0.53). Point kubelet's
  `resolvConf` at `/run/systemd/resolve/resolv.conf` (Ansible kubeadm role).
- Pods Running but no endpoints: check readiness and NetworkPolicies in
  `kube-system`.

## 3. Upstream resolution failing

```bash
kubectl -n kube-system get cm coredns -o jsonpath='{.data.Corefile}' | grep -A2 forward
ssh <node> 'resolvectl status | head -30; dig +short ghcr.io @1.1.1.1'
```

If the node itself cannot resolve, fix node networking (gateway, firewall,
security group egress on UDP/TCP 53). As a temporary measure, forward to
public resolvers in the Corefile via Git:

```
forward . 1.1.1.1 9.9.9.9 { max_concurrent 1000 }
```

## 4. Some pods only: NetworkPolicy or Cilium

Every namespace's default-deny policy must allow egress to `kube-dns` on
UDP/TCP 53. Check for drops:

```bash
hubble observe --namespace satark --protocol dns --verdict DROPPED --last 50
hubble observe --to-namespace kube-system --to-label k8s-app=kube-dns --verdict DROPPED --last 50
kubectl -n satark get networkpolicy,ciliumnetworkpolicy
```

If drops are on one node only, restart that node's Cilium agent
(`kubectl -n kube-system delete pod <cilium-xxxxx>`); see RB-002 section 4.

## 5. Slow DNS

- Scale CoreDNS: `kubectl -n kube-system scale deploy/coredns --replicas=3`
  (make it permanent in Git).
- `ndots:5` turns every external lookup into up to 5 queries. Services that
  call external hosts should use fully qualified names with a trailing dot or
  set `dnsConfig.options: [{name: ndots, value: "2"}]` in their chart.
- Check the `cache` plugin is present in the Corefile.

## 6. Verify

- The debug pod resolves cluster and external names in under 100 ms.
- CoreDNS SERVFAIL ratio in the Cluster health dashboard is back near zero.
- `url-intel` error ratio in the Scan pipeline dashboard has recovered.
- Alerts resolved in the Telegram ops channel.

## 7. Follow-up

If a ConfigMap or NetworkPolicy change caused this, add a test or policy check
that would have caught it.
