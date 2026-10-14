# ADR-001: kubeadm instead of k3s for the on-prem cluster

- **Status:** Accepted
- **Date:** 2026-10-14
- **Deciders:** Satark maintainers

## Context

Satark's platform layer runs on a small on-prem cluster: three Ubuntu VMs
(`cp-1`, `worker-1`, `worker-2`) on a laptop or home server with 8–16 GB of RAM.
The cluster must:

- behave like a production upstream Kubernetes cluster, because a stated goal
  of the project is to practise the operations a real infrastructure team does
  (minor-version upgrades, etcd backups, certificate rotation, node add/remove,
  API audit logging);
- be fully reproducible from code (Vagrant + Ansible) with no manual steps;
- run Cilium as the CNI with kube-proxy replacement (see ADR-003);
- fit in the RAM budget alongside Postgres, Valkey, NATS and the observability
  stack.

The two realistic options for a self-built cluster were **kubeadm** and **k3s**.
(Managed Kubernetes is out of scope for the on-prem layer; RKE2, Talos and
MicroK8s were considered briefly and are covered below.)

## Decision

Build the on-prem cluster with **kubeadm**, driven by the Ansible roles
`k8s_prereqs`, `containerd`, `kubeadm_cp` and `kubeadm_worker`, using the
upstream `pkgs.k8s.io` packages pinned to the N-1 minor version.

k3s stays in use for one narrow case: the temporary single-VM disaster
recovery target in Phase 11, where startup speed matters more than fidelity.

## Alternatives considered

| Option | Pros | Cons |
|---|---|---|
| **kubeadm** (chosen) | Upstream reference installer; same components, flags and file layout as most production clusters; static-pod control plane with stacked etcd that can be backed up with `etcdctl`; first-class `kubeadm upgrade` workflow; explicit configuration of audit logging, admission and certificates | More moving parts to automate (container runtime, CNI, kubelet config, join tokens); slightly higher memory use; no bundled ingress, storage or load balancer |
| **k3s** | Single binary, very fast install, low memory; bundles Traefik, ServiceLB, local-path storage and flannel | Diverges from upstream (SQLite/kine or embedded etcd, combined process, different file paths and flags); upgrades are a binary swap, so they do not exercise the `kubeadm upgrade` + drain procedure; bundled components have to be disabled to use Cilium and MetalLB; skills transfer less directly to production clusters |
| RKE2 | Hardened defaults (CIS), upstream-like | Heavier, opinionated packaging, smaller community for troubleshooting |
| Talos Linux | Immutable, API-driven, very secure | Replaces the OS, so the Ansible hardening roles and SSH-based operations the project wants to practise do not apply |
| MicroK8s | Easy snap install | Snap-based lifecycle, add-on model differs from upstream |

## Consequences

**Positive**

- Runbooks and SOPs (upgrade, etcd backup, certificate expiry, node add/remove)
  use the same commands as production upstream clusters.
- API server audit logging, kubelet serving-certificate bootstrap and
  `protectKernelDefaults` are configured explicitly in `kubeadm-config.yaml`
  and reviewed in Git.
- The N-1 version pin makes a real `N-1 → N` upgrade possible under load
  (Phase 11).

**Negative**

- We own more automation: container runtime, CNI, MetalLB, metrics-server and
  storage are separate roles/add-ons instead of bundled.
- The control plane uses more memory than k3s (roughly 1–1.5 GB on `cp-1`),
  which is why the recommended host has 16 GB of RAM; an 8 GB host runs one
  worker (`SATARK_WORKERS=1`).
- A single control-plane node is not highly available. This is accepted for a
  lab; etcd snapshots to S3 (Phase 11) and the DR drill cover recovery.

## References

- kubeadm documentation: <https://kubernetes.io/docs/reference/setup-tools/kubeadm/>
- k3s documentation: <https://docs.k3s.io/>
- `infra/ansible/roles/kubeadm_cp`, `infra/ansible/roles/kubeadm_worker`
