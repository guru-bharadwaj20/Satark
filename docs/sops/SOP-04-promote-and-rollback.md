# SOP-04: Promote a release to prod and roll back

| | |
|---|---|
| **Owner** | Release maintainer (code owner of `gitops/envs/prod/`) |
| **When** | Shipping what is running in `dev` to `prod`, or undoing a prod release |
| **Time** | About 15 minutes to promote; under 5 minutes to roll back |
| **Related** | ADR-007, `.github/workflows/promote.yml`, `scripts/rollback-drill.sh` |

## Before you start

- [ ] The tags you want have run in `dev` for at least 30 minutes with no new
      alerts in Alertmanager and no error-rate rise in Grafana.
- [ ] CI on `main` is green (lint, tests, Trivy, cosign).
- [ ] You have `kubectl` access to the cluster (read is enough) and the
      `argocd` CLI or UI.
- [ ] No other promotion PR is open.

## Promote

1. **Open the promotion PR.** GitHub, Actions, `promote`, Run workflow.
   Leave *services* empty to promote everything that differs, or list them
   (`gateway-api,url-intel`). Fill in *reason*.
2. **Review the PR.** Check the before/after tag table. For each new tag,
   verify the signature:

   ```bash
   cosign verify ghcr.io/guru-bharadwaj20/satark/<service>:<tag> \
     --certificate-identity-regexp '^https://github.com/guru-bharadwaj20/satark/.github/workflows/build.yml@refs/heads/main$' \
     --certificate-oidc-issuer https://token.actions.githubusercontent.com
   ```

3. **Approve and merge.** A code owner other than the person who ran the
   workflow approves. Use *Create a merge commit* or *Squash*, and note the
   resulting commit SHA.
4. **Watch the sync.**

   ```bash
   argocd app get <service>-prod --refresh
   kubectl -n satark rollout status deploy/<service> --timeout=5m
   ```

5. **Verify.** `/readyz` is 200, the Grafana service dashboard shows no error
   rise for 10 minutes, and a smoke check against the public API passes.
6. **Record** the PR link and result in the sprint notes.

## Roll back

Roll back as soon as prod is worse than before; investigate afterwards.

1. **Revert the promotion commit** (fastest, keeps history):

   ```bash
   git switch main && git pull
   git revert --no-edit <promotion-merge-sha>     # add -m 1 for a merge commit
   git push origin HEAD:main                      # or open a PR if push is blocked
   ```

   If branch protection blocks direct pushes, open the revert as a PR and get
   an emergency approval; mention "rollback" in the title.
2. **Force a refresh** so you do not wait for the 3-minute poll:

   ```bash
   argocd app get <service>-prod --refresh
   ```

3. **Confirm** the Deployment runs the previous tag and is healthy:

   ```bash
   kubectl -n satark get deploy <service> -o jsonpath='{.spec.template.spec.containers[0].image}'
   kubectl -n satark rollout status deploy/<service>
   ```

4. **Do not** use `argocd app rollback` or `kubectl rollout undo` as the fix:
   self-heal puts back whatever Git says. They are acceptable only as a
   stopgap while the revert is merging, with auto-sync paused
   (`argocd app set <app> --sync-policy none`), and auto-sync must be
   re-enabled afterwards.
5. **Follow up:** open an incident note, keep `dev` on the bad tag (or revert
   it too) until the cause is fixed.

## If Argo CD itself is broken

`argocd` Application has prune disabled, so a bad change to
`gitops/platform/argocd` cannot delete it. Revert the change in Git, then if
Argo CD cannot sync itself, rerun the bootstrap role:

```bash
ansible-playbook -i infra/ansible/inventory infra/ansible/playbooks/argocd.yml
```
