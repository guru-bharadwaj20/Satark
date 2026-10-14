# SOP-02: Add or remove a node

| | |
| --- | --- |
| Owner | Platform on-call |
| Duration | About 20 minutes per node |
| Risk | Low for add, medium for remove (capacity drops) |
| Automation | `infra/ansible/playbooks/add-node.yml`, `remove-node.yml` |

Only worker nodes are covered. The control plane is a single kubeadm node;
replacing it is a disaster-recovery action (SOP-05).

## Add a worker

### Prerequisites

- Ubuntu 24.04 VM with at least 4 vCPU, 8 GiB RAM, 60 GiB disk, a static IP
  on the cluster subnet and SSH access for the `ansible` user.
- Hostname is unique and follows `worker-N`.

### Steps

1. Add the host to the inventory under **both** `workers` and `new_nodes`:
   ```ini
   [workers]
   worker-1 ansible_host=10.20.0.21
   worker-2 ansible_host=10.20.0.22
   worker-3 ansible_host=10.20.0.23

   [new_nodes]
   worker-3
   ```
2. Dry run, then apply:
   ```bash
   cd infra/ansible
   ansible-playbook -i inventory/prod playbooks/add-node.yml -l 'new_nodes:control_plane[0]' --check
   ansible-playbook -i inventory/prod playbooks/add-node.yml -l 'new_nodes:control_plane[0]'
   ```
3. Verify:
   ```bash
   kubectl get node worker-3 -o wide          # Ready, same kubelet version
   kubectl -n kube-system get pods -o wide --field-selector spec.nodeName=worker-3   # cilium, kube-proxy
   kubectl satark doctor
   ```
4. Remove the host from `new_nodes` in the inventory and commit.
5. Optional: rebalance with `kubectl rollout restart deploy -n satark`.

## Remove a worker

### Before you start

- Check spare capacity. After removal, the remaining nodes must fit all
  requests: `kubectl describe nodes | grep -A5 "Allocated resources"`.
- Check no PDB will block the drain: `kubectl get pdb -A` (ALLOWED DISRUPTIONS > 0).
- If the node hosts the CNPG primary, switch over first:
  `kubectl cnpg promote satark-pg <replica> -n satark-data`.

### Steps

1. Run:
   ```bash
   ansible-playbook -i inventory/prod playbooks/remove-node.yml -e node=worker-3
   ```
   The playbook refuses to remove a control-plane node or the last worker.
2. If the drain hangs, look for a pod stuck on a PDB:
   `kubectl get pods -A -o wide --field-selector spec.nodeName=worker-3`.
   Do **not** use `--disable-eviction` unless the incident commander agrees.
3. Delete the host from the inventory and commit.
4. Release the VM (Vagrant: `vagrant destroy worker-3`).
5. Verify `kubectl get nodes` and `kubectl satark doctor`.

## Rollback

- Failed add: run `remove-node.yml -e node=<host>` to reset it, fix the cause,
  then add again.
- Removed the wrong node: re-add it with the add procedure. Workloads will
  reschedule on their own.
