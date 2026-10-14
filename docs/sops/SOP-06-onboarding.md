# SOP-06: Onboarding, from zero to a running cluster

| | |
|---|---|
| **Purpose** | Take a new contributor from a bare Windows (or Linux) machine to a working three-node Satark cluster |
| **Audience** | New contributors and operators |
| **Time** | About 60–90 minutes, mostly downloads and VM boot time |
| **Prerequisites** | 16 GB RAM (8 GB works with one worker), 4+ CPU cores with virtualisation enabled in the BIOS, 40 GB of free disk, admin rights, internet access |

## 1. Get the code

```bash
git clone https://github.com/guru-bharadwaj20/satark.git
cd satark
```

On Windows, clone **inside WSL** (step 3) rather than onto `C:\`, so that file
permissions work for Ansible and SSH.

## 2. Install host tools

### Windows

From an elevated PowerShell prompt in the repository:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\scripts\setup-host.ps1
```

This installs VirtualBox, Vagrant and Git with winget and enables WSL2 with
Ubuntu 24.04. If the script asks for a reboot, reboot and run it again.

> If Hyper-V or Windows Sandbox is enabled, VirtualBox runs much more slowly.
> Disable them if `vagrant up` is very slow.

### Linux

Install VirtualBox, Vagrant and Git from your distribution or the vendors'
repositories, then go to step 3 and run the control-node script on the host.

## 3. Set up the Ansible control node

Open the Ubuntu (WSL) terminal and run:

```bash
cd ~/satark            # your clone inside WSL
./scripts/setup-control-node.sh
exec "$SHELL" -l       # pick up PATH changes
```

This installs `ansible-core`, `ansible-lint`, the Ansible collections,
`kubectl` and `helm`, and creates the SSH key `~/.ssh/satark_ed25519` that
Ansible uses to reach the VMs.

If the script changed `/etc/wsl.conf`, run `wsl --shutdown` from PowerShell,
then reopen Ubuntu.

## 4. Build the cluster

```bash
./scripts/bootstrap.sh
```

On an 8 GB host, use a single worker:

```bash
SATARK_WORKERS=1 ./scripts/bootstrap.sh
```

Remove `worker-2` from `infra/ansible/inventory/onprem/hosts.yml` first in that case.

What happens:

1. Vagrant creates `cp-1` (192.168.56.10), `worker-1` (.11) and `worker-2` (.12).
2. `infra/ansible/playbooks/site.yml` runs the roles in order: `common`,
   `hardening`, `k8s_prereqs`, `containerd`, `kubeadm_cp`, `kubeadm_worker`,
   `cilium`, `addons` (MetalLB, metrics-server, local-path-provisioner).
3. The admin kubeconfig is written to `~/.kube/satark-onprem.conf`.

To re-run Ansible without touching the VMs:

```bash
./scripts/bootstrap.sh --skip-vagrant
./scripts/bootstrap.sh --skip-vagrant -- --tags addons   # one stage only
```

## 5. Use the cluster

```bash
export KUBECONFIG=~/.kube/satark-onprem.conf
kubectl get nodes -o wide
kubectl get pods -A
```

SSH to a node:

```bash
ssh -i ~/.ssh/satark_ed25519 vagrant@192.168.56.10
# or, from infra/vagrant:  vagrant ssh cp-1
```

Password and root logins are disabled by the `hardening` role.

Hubble UI:

```bash
kubectl -n kube-system port-forward svc/hubble-ui 12000:80
# open http://localhost:12000
```

## 6. Daily operation

| Task | Command (from `infra/vagrant`) |
|---|---|
| Stop the lab | `vagrant halt` |
| Start it again | `vagrant up`, then `kubectl get nodes` |
| Destroy everything | `vagrant destroy -f` |
| Rebuild from scratch | `vagrant destroy -f && ../../scripts/bootstrap.sh` |

## Troubleshooting

| Symptom | Fix |
|---|---|
| `vagrant up` fails with `VT-x is not available` | Enable virtualisation in the BIOS; disable Hyper-V (`bcdedit /set hypervisorlaunchtype off`) and reboot |
| Vagrant under WSL cannot find VirtualBox | Open a new shell after `setup-control-node.sh`; check `VAGRANT_WSL_ENABLE_WINDOWS_ACCESS=1` and that `/mnt/c/Program Files/Oracle/VirtualBox` is on `PATH` |
| Ansible ignores `ansible.cfg` ("world writable directory") | The repo is on `/mnt/c`. Clone it inside the WSL filesystem, or apply the `wsl.conf` metadata option and restart WSL |
| `UNREACHABLE` for a node | `vagrant status`; `ssh -i ~/.ssh/satark_ed25519 vagrant@<ip>`; make sure the public key was present before `vagrant up` (re-run `vagrant provision`) |
| Nodes stay `NotReady` | Cilium is not up yet: `kubectl -n kube-system get pods -l k8s-app=cilium`; re-run `--tags cilium` |
| LoadBalancer stays `<pending>` | `kubectl -n metallb-system get pods,ipaddresspools,l2advertisements`; re-run `--tags addons` |
| `kubectl top nodes` errors | Pending kubelet serving CSRs: `kubectl get csr`, then `kubectl certificate approve <name>` |

## Verification

Run the smoke test:

```bash
./scripts/verify-cluster.sh
```

Expected result: every check shows `[PASS]` and the script ends with
`Cluster verification PASSED`. In particular:

- all nodes are `Ready`;
- `kubernetes.default.svc.cluster.local` and an external name resolve from a pod;
- a test LoadBalancer Service gets an IP in `192.168.56.240–192.168.56.250` and answers HTTP;
- `kubectl top nodes` works and `local-path` is the default StorageClass.

If any check fails, use the troubleshooting table above, then ask in the project
discussions with the script output attached.
