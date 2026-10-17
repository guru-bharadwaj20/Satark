# Sandbox SSRF verification, 2026-10-17

Run id: `ssrf-1792226090`, cluster: `kubernetes-admin@satark-onprem`,
in-cluster service target: `satark-dev/gateway-api`

| Target | URL | Hubble DROPPED | Hubble FORWARDED | Scan phase | Result |
|---|---|---|---|---|---|
| metadata | `http://169.254.169.254/latest/meta-data/` | 20 | 0 | TimedOut | PASS |
| apiserver | `https://10.96.0.1/` | 20 | 0 | TimedOut | PASS |
| service | `http://10.109.91.85:8080/` | 20 | 0 | TimedOut | PASS |
| rfc1918 | `http://192.168.1.1/` | 20 | 0 | TimedOut | PASS |

Control scan (`https://example.com/`): **Succeeded**

Overall: **PASS**

Produced by `scripts/verify-sandbox-ssrf.sh --report` (task 8.20), which
checks the policies from task 8.19 (`gitops/platform/sandbox/networkpolicy.yaml`).

## What the drops were

Every drop was an egress SYN from the sandbox pod, dropped with Hubble drop
reason `POLICY_DENY` (code 181). That is the code for an explicit deny rule,
so the drops came from the `sandbox-deny-internal` CiliumNetworkPolicy, not
just from the NetworkPolicy's default deny. Chromium retried each connection
until the scan's 30 s deadline, which is why each internal scan ended
`TimedOut` with no page loaded.

| Destination | Drop reason | Direction | Service |
|---|---|---|---|
| `169.254.169.254:80` | POLICY_DENY | EGRESS | – |
| `10.96.0.1:443` | POLICY_DENY | EGRESS | `default/kubernetes` |
| `10.109.91.85:8080` | POLICY_DENY | EGRESS | `satark-dev/gateway-api` |
| `192.168.1.1:80` | POLICY_DENY | EGRESS | – |

Hubble records ClusterIP drops under the service IP and names the service.
The policy is applied after Cilium resolves the service, and the destination
is recorded before translation, so `--to-ip <ClusterIP>` and the script's
destination match both work.

## Setup

| Component | Version / setting |
|---|---|
| Cluster | on-prem lab, kubeadm v1.35.9, containerd 2.2.1, Ubuntu kernel 6.8 |
| CNI | Cilium 1.18.2 (kube-proxy replacement), Hubble relay |
| Hubble CLI | v1.18.6 (sha256 checked), relay on `localhost:4245` via `kubectl port-forward` |
| gVisor | `release-20260928.0`, `systrap` platform, from `playbooks/gvisor.yml` |
| Sandbox pods | RuntimeClass `gvisor` (handler `runsc`), PSA `restricted`, DNS 1.1.1.1 / 9.9.9.9 |
| Operator | `apps/linkscan-operator/config/lab`: no NATS, Prometheus or KEDA |

```
ansible-playbook playbooks/gvisor.yml                  # from infra/ansible
kubectl apply -f <satark-sandbox Namespace from gitops/platform/namespaces>
kubectl apply -k gitops/platform/sandbox
IMAGE_TAG=lab NODES="worker-1 worker-2" scripts/load-lab-images.sh linkscan-operator sandbox-browser
kubectl apply --server-side -k apps/linkscan-operator/config/lab
kubectl -n kube-system port-forward svc/hubble-relay 4245:80 &
scripts/verify-sandbox-ssrf.sh --report docs/compliance/sandbox-ssrf-2026-10-17.md
```

## Issues found and fixes

- **The script could not have passed.** Hubble refuses `--to-ip` together
  with `--namespace`, which the old script used for every target. The new
  script records the namespace's flows with `--follow` while the scans run
  and counts drops per destination from that recording.
- **Hubble's buffer covers only about 80 seconds.** The buffer was full at
  12,285 flows and the cluster ran about 157 flows/s, so a query made after
  the scans finish could miss the first drops. Recording during the run
  avoids that.
- **The NATS target fell back to a port nothing serves.** NATS is not
  installed in the lab, and the fallback, port 8222 on kube-dns's IP, is not
  a real service. The script now targets an in-cluster HTTP service:
  `satark-data/nats` if it exists, otherwise the gateway API.
- **The gVisor role had never run.** No playbook used it, and it failed on
  current gVisor and containerd:
  - Releases now ship one `gvisor.tar.bz2`, which includes the
    `gvisor-bin/` sidecars that `runsc` needs to start.
  - containerd 2.x needs the `io.containerd.cri.v1.runtime` plugin path. The
    role now reads the major version from `containerd --version`.
  - The smoke test looked for `dmesg` output, but `ctr run` shows no stdout
    from the runsc shim. It now checks the exit code instead: the container
    must report gVisor's kernel version, not the host's.
  - The new `playbooks/gvisor.yml` also labels the workers
    `satark.io/gvisor=true`, which the RuntimeClass schedules on.
- **The sandbox-browser image did not build.** `playwright install
  --only-shell` needs Playwright 1.49 or later, and the image pinned 1.48.0.
  It now pins 1.55.0. A public scan of `https://example.com/` ran under
  gVisor in 9 s, with title, redirect chain and forms reported.
