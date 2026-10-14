# ADR-006: Sandbox isolation model

- **Status:** Accepted
- **Date:** 2026-10-14
- **Deciders:** Satark maintainers

## Context

To check a suspicious link, Satark opens it in a real browser (`sandbox-browser`,
headless Chromium). That means running attacker-controlled HTML and JavaScript
inside our on-prem cluster. The risks are:

1. **Container escape:** a Chromium or kernel exploit gets out of the pod onto the node.
2. **SSRF and lateral movement:** the page makes the browser call internal
   addresses: the Kubernetes API, Postgres, Valkey, NATS, the node, or the cloud
   metadata endpoint `169.254.169.254`.
3. **Resource abuse:** crypto-miners, fork bombs, huge downloads, never-ending pages.
4. **Credential theft:** anything mounted in the pod (service account tokens, cloud keys).
5. **Data exposure:** links can carry personal data in query strings.

## Decision

Defence in depth, with every layer assuming the one before it has failed:

| Layer | Control |
|---|---|
| Kernel | Pods run under **gVisor** (`runsc`, RuntimeClass `gvisor`). Syscalls are handled by the user-space Sentry, not the host kernel. Chromium's own sandbox is turned off (`--no-sandbox`) because it needs privileges we refuse to grant, so gVisor is the main boundary. |
| Pod | One short-lived pod per scan, created by `linkscan-operator`. Non-root UID 10001, read-only root filesystem, all capabilities dropped, `allowPrivilegeEscalation: false`, seccomp `RuntimeDefault`, no service account token, no service links, `restartPolicy: Never`. |
| Network | `NetworkPolicy` + `CiliumNetworkPolicy`: no ingress; egress only to public IPs on web ports. RFC 1918, CGNAT, loopback, link-local (metadata), and the pod/service CIDRs are denied, with Cilium deny rules that later allow policies cannot override. Pods use `dnsPolicy: None` with public resolvers, so cluster service names do not resolve. Checked with Hubble (`scripts/verify-sandbox-ssrf.sh`). |
| Time and resources | `activeDeadlineSeconds` (default 60 s, max 300 s) plus an operator-side hard deadline; CPU, memory and ephemeral-storage limits; size-limited `emptyDir` for `/tmp` and `/dev/shm`; downloads are never saved. |
| Placement | Only nodes labelled `satark.io/gvisor=true` run sandbox pods, in the dedicated `satark-sandbox` namespace. |
| Results | The pod has no route to NATS or the API. It returns a small JSON result through its termination message (≤ 4 KiB), which the operator reads and publishes. |
| Lifetime | `LinkScan` objects and pods are deleted after a TTL (default 1 h). Screenshots go to S3 under `screenshots/`, with a lifecycle rule. |

The only secret a sandbox pod can see is an optional S3 key limited to
`s3:PutObject` on the `screenshots/` prefix. That is the one credential an
exploit could steal, and all it allows is uploading objects.

## Alternatives considered

- **Plain runc + seccomp/AppArmor:** much less isolation. One kernel bug in a
  syscall Chromium needs would be enough to escape.
- **Kata Containers / Firecracker microVMs:** stronger isolation, but they need
  nested virtualisation on our on-prem nodes and a heavier footprint per scan.
  We can revisit this if gVisor compatibility or performance becomes a problem.
- **A third-party scanning API (urlscan.io etc.):** sends user links, which may
  contain personal data, to an outside service, adds cost and rate limits, and
  teaches us nothing about running untrusted workloads.
- **A long-running pool of browsers:** faster, but state leaks between scans
  (cookies, cache, compromised processes). One fresh pod per scan is simpler to
  reason about.

## Consequences

- Startup is slower (about 1–3 s for gVisor and Chromium) and syscall-heavy work
  costs more CPU. We accept that for a background check, because the bot replies
  first and edits its reply when the sandbox result arrives.
- Some sites behave differently under gVisor or headless Chromium, and some block
  data-centre IPs. A "Failed" or "TimedOut" scan is shown to users as "not
  checked", never as "safe".
- Nodes need the `gvisor` Ansible role. If a node lacks it, the RuntimeClass
  `nodeSelector` keeps sandbox pods off that node.
- Capacity scales with KEDA on the length of the NATS `SCAN_JOBS` queue
  (RB-007). Sandbox failures are covered in RB-008.
