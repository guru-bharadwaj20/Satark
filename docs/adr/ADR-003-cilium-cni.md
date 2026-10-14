# ADR-003: Cilium as the CNI

- **Status:** Accepted
- **Date:** 2026-10-14
- **Deciders:** Satark maintainers

## Context

The on-prem kubeadm cluster (ADR-001) needs a CNI plugin. Satark's needs:

- **Strict NetworkPolicy.** The link sandbox (Phase 8) runs untrusted pages and
  must be limited to egress-only internet access with no route to cluster
  services. Application namespaces get default-deny policies (Phase 9). Some of
  these rules need FQDN or L7 awareness, which standard Kubernetes
  NetworkPolicy cannot express.
- **Observability of network flows.** When a policy breaks traffic (game day 4:
  "apply a bad NetworkPolicy"), operators need to see which flows were dropped
  and why, without tcpdump on every node.
- **Low overhead** on a 3-VM lab with limited CPU and RAM.
- **Works with MetalLB** for LoadBalancer Services on the host-only network.

## Decision

Use **Cilium** (installed by the `cilium` Ansible role with Helm) with:

- **kube-proxy replacement**: kubeadm skips the `kube-proxy` add-on and Cilium
  handles Services with eBPF;
- **VXLAN tunnel routing** and **cluster-pool IPAM** over `10.244.0.0/16`;
- **Hubble**, Hubble Relay and Hubble UI enabled, with flow, DNS, drop and HTTP
  metrics exported to Prometheus.

MetalLB (L2 mode) still assigns and announces LoadBalancer IPs. Cilium's own
L2 announcements and LB-IPAM are not used for now, to keep the address
management separate from the CNI and simple to debug.

## Alternatives considered

| Option | Pros | Cons |
|---|---|---|
| **Cilium** (chosen) | eBPF data path; Kubernetes NetworkPolicy plus `CiliumNetworkPolicy` with FQDN and L7 rules; Hubble flow visibility and metrics; can replace kube-proxy; widely used in production; CNCF graduated | More complex than simple CNIs; needs a recent kernel (Ubuntu 24.04 is fine); agent uses about 150–300 MB of RAM per node |
| Calico | Mature, solid NetworkPolicy, BGP support, eBPF mode available | FQDN policies and flow logs are mostly in the commercial tiers; no built-in equivalent to Hubble in the open-source version |
| Flannel | Very simple and light | No NetworkPolicy support at all, so it fails the sandbox requirement |
| Canal (Flannel + Calico policy) | Simple networking with policy | Two projects to run and debug; no L7/FQDN policy or flow visibility |
| Weave Net | Easy setup | Project archived; not suitable for new clusters |

## Consequences

**Positive**

- One tool covers networking, Service load balancing, policy enforcement and
  flow observability.
- `hubble observe --verdict DROPPED` shows dropped flows during incidents and
  game days; Hubble metrics feed the Grafana dashboards in Phase 6.
- FQDN-aware egress policies make the sandbox egress rules precise.
- No kube-proxy means no large iptables rule sets on the nodes.

**Negative**

- The host firewall (`hardening` role) has to allow Cilium ports (VXLAN 8472/udp,
  health 4240, Hubble 4244) and accept traffic on the `cilium_*` and `lxc*`
  interfaces. These are documented in `roles/hardening/defaults/main.yml`.
- With kube-proxy replacement, the API server address has to be given to
  Cilium explicitly (`k8sServiceHost`), and the agent must come up before
  CoreDNS can run.
- Strict reverse-path filtering breaks Cilium routing, so `rp_filter` is set to
  loose mode (2) in the sysctl hardening.
- Cilium upgrades need care and follow the upstream upgrade guide (pre-flight
  checks, one minor version at a time).

## References

- Cilium documentation: <https://docs.cilium.io/>
- Kube-proxy replacement: <https://docs.cilium.io/en/stable/network/kubernetes/kubeproxy-free/>
- `infra/ansible/roles/cilium`
