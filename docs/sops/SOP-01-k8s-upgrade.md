# SOP-01: Kubernetes minor-version upgrade

| | |
| --- | --- |
| Owner | Platform on-call |
| Frequency | Every minor release, within 60 days of it reaching `.2` |
| Duration | 45 to 90 minutes for three nodes |
| Risk | Medium. One node is out of service at a time. |
| Automation | `infra/ansible/playbooks/upgrade.yml` (role `k8s_upgrade`) |

kubeadm supports moving exactly **one minor version** at a time (for example
1.30 to 1.31). To go further, repeat this SOP once per minor.

## 1. Preparation (the day before)

1. Read the upstream changelog "Urgent Upgrade Notes" for the target minor and
   search it for APIs we use. Check deprecated APIs in the cluster:
   ```bash
   kubectl get --raw /metrics | grep apiserver_requested_deprecated_apis
   ```
2. Confirm platform add-ons support the target minor: Cilium, MetalLB,
   Envoy Gateway, cert-manager, KEDA, CNPG, Argo CD, Velero, kube-prometheus-stack.
   Upgrade any that do not **before** this SOP.
3. Make sure every Deployment with more than one replica has a
   PodDisruptionBudget, and that no PDB has `ALLOWED DISRUPTIONS` of 0.
   The exception is CNPG's `<cluster>-primary` PDB, which is always 0 by design:
   the playbook switches the primary off a node before draining it and puts
   the cluster into node maintenance so replicas on local storage do not
   block the drain (`roles/k8s_upgrade/tasks/cnpg.yml`):
   ```bash
   kubectl get pdb -A
   kubectl satark doctor
   ```
4. Announce a maintenance window in the status channel.

## 2. Backups (start of window)

1. Trigger and verify an etcd snapshot (the playbook does this too):
   ```bash
   ssh cp1 sudo systemctl start etcd-backup.service
   aws s3 ls s3://satark-backups-prod/etcd/cp1/ | tail -2
   ```
2. Trigger a Velero backup and wait for `Completed`:
   ```bash
   velero backup create pre-upgrade-$(date +%Y%m%d) --from-schedule daily --wait
   ```
3. Confirm the last CNPG backup is recent:
   `kubectl -n satark-data get backups.postgresql.cnpg.io --sort-by=.metadata.creationTimestamp | tail -1`.

## 3. Upgrade

1. Start the load generator in a separate terminal (optional but recommended):
   `tests/load/upgrade-under-load.sh` does both steps 1 and 2 together.
2. Run the playbook from `infra/ansible`:
   ```bash
   ansible-playbook -i inventory/prod playbooks/upgrade.yml -e k8s_upgrade_target_minor=1.31
   ```
   The playbook:
   - refuses to run unless the target is exactly one minor above the cluster
     and every node is Ready;
   - switches the apt repo to `pkgs.k8s.io/core:/stable:/v1.31`;
   - on the first control plane runs `kubeadm upgrade plan` and
     `kubeadm upgrade apply`, then drains, upgrades kubelet and uncordons;
   - for each worker (`serial: 1`): drain, `kubeadm upgrade node`, kubelet,
     uncordon, wait for pods to schedule;
   - around every drain: CNPG node maintenance on, primary switched to a
     replica on another node, and afterwards maintenance off and a wait until
     every instance is ready again.
3. Watch while it runs:
   ```bash
   watch kubectl get nodes -o wide
   kubectl get pods -A --field-selector status.phase!=Running,status.phase!=Succeeded
   ```

## 4. Verification

- [ ] `kubectl get nodes` shows the new version on every node, all Ready.
- [ ] `kubectl satark doctor` reports no failures.
- [ ] Argo CD: every Application `Synced` and `Healthy`.
- [ ] Grafana "Satark SLO" dashboard: no error-budget burn alert fired.
- [ ] k6 summary error count recorded in `docs/game-days/upgrade-under-load.md`.

## 5. Rollback

kubeadm cannot downgrade a control plane in place.

- **A worker failed mid-upgrade:** fix forward on that node (re-run the
  playbook with `--limit <node>`). If the node is broken, remove it with
  `playbooks/remove-node.yml` and add a fresh one with `playbooks/add-node.yml`
  (SOP-02).
- **The control plane is broken after `upgrade apply`:** kubeadm keeps the
  previous static Pod manifests in `/etc/kubernetes/tmp/kubeadm-backup-manifests-*`.
  Copy them back to `/etc/kubernetes/manifests/` and reinstall the old
  kubelet/kubeadm packages. If etcd is damaged, restore the pre-upgrade
  snapshot: `etcdutl snapshot restore` then point the etcd manifest at the
  restored data dir (see RB-006).
- **Total loss:** follow SOP-05 (disaster recovery).

## 6. Afterwards

- Update `k8s_version` in the inventory group vars so new nodes join on the
  new version.
- Record the run in `docs/game-days/upgrade-under-load.md`.
