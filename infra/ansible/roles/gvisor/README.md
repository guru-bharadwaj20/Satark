# Role: gvisor

Installs gVisor's `runsc` and `containerd-shim-runsc-v1` (checksums verified
against the release `.sha512` files) and registers a `runsc` runtime handler in
containerd. Sandbox pods select it through the `gvisor` RuntimeClass.

Run it on worker nodes that host `satark-sandbox` pods, after the containerd role:

```yaml
- hosts: k8s_workers
  become: true
  roles:
    - gvisor
```

Key variables: `gvisor_release`, `gvisor_platform` (`systrap` by default; `kvm`
needs nested virtualisation), `gvisor_containerd_major` (set to `2` on containerd 2.x).
