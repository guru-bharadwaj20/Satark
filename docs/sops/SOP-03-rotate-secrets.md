# SOP-03: Rotate secrets and credentials

| | |
|---|---|
| **When** | On the schedule in `gitops/envs/secrets-inventory.yaml` (`rotation`), after a person with access leaves, or right away if a leak is suspected (a gitleaks alert, an exposed-secret report or an Access Analyzer finding) |
| **Who** | One on-call engineer with cluster-admin access and AWS IAM admin access |
| **Time** | About 15 minutes per secret |
| **Risk** | Medium. Consumers that are not restarted keep the old value until they are |

Rotation follows the same order every time: **create the new value, deploy it, verify, then revoke the old one.** Never revoke first, except when a leak is confirmed (see section 6).

## 0. Prerequisites

```bash
kubectl config current-context        # should be the target cluster
kubeseal --version
aws sts get-caller-identity           # AWS-backed secrets only
export ENV=dev                        # or prod
export VALUES=~/satark-secrets/$ENV   # local, git-ignored, on an encrypted disk
```

## 1. Application secrets (Telegram, API keys, LLM, feeds)

1. Generate or obtain the new value:
   - Telegram bot token: in @BotFather, run `/revoke` and pick the bot. **This revokes the old token immediately**, so do it in a maintenance window. To change only the webhook secret, run `openssl rand -hex 32`.
   - HMAC and webhook secrets: `openssl rand -hex 32`.
   - Third-party API keys: create a second key in the provider console and keep the old one active for now.
2. Update `$VALUES/<name>.env`.
3. Seal it:
   ```bash
   scripts/seal-secret.sh satark <name> gitops/envs/$ENV/secrets/<name>.sealed.yaml < $VALUES/<name>.env
   ```
4. Open a PR with only the changed `*.sealed.yaml`. The gitleaks and secrets-guard checks must pass. Merge it.
5. Wait for Argo CD to sync, then restart the consumers listed in the inventory:
   ```bash
   argocd app wait satark-$ENV --health --timeout 300
   kubectl -n satark rollout restart deploy/<consumer>
   kubectl -n satark rollout status deploy/<consumer> --timeout=180s
   ```
6. Revoke the old key in the provider console.

## 2. Postgres (CNPG) application user

```bash
NEWPW="$(openssl rand -base64 30 | tr -d '/+=')"
printf 'username=satark\npassword=%s\n' "$NEWPW" > $VALUES/satark-pg-app.env
scripts/seal-secret.sh satark-data satark-pg-app gitops/envs/$ENV/secrets/satark-pg-app.sealed.yaml < $VALUES/satark-pg-app.env
unset NEWPW
```

Merge the PR. CNPG watches the referenced secret and runs `ALTER ROLE` itself. Then restart `gateway-api` and `reputation`.

## 3. Valkey and NATS passwords

Both servers need the new password on the server and the clients at the same moment. To keep this to a single short restart:

1. Seal the new `valkey-auth` / `nats-auth` secrets and merge them.
2. Restart the server first, then each client:
   ```bash
   kubectl -n satark-data rollout restart statefulset/valkey statefulset/nats
   kubectl -n satark-data rollout status statefulset/valkey --timeout=300s
   kubectl -n satark rollout restart deploy/gateway-api deploy/url-intel deploy/reputation deploy/sandbox-browser
   ```
3. Expect a burst of errors for under a minute. The `satark_` error-rate alerts may fire once.

## 4. AWS access keys (Velero and CNPG backup users)

These are the IAM users from `infra/terraform/modules/backup-iam`. An IAM user can have two keys, so rotation has no downtime.

```bash
USER=satark-$ENV-velero-backup          # or satark-$ENV-cnpg-backup
aws iam list-access-keys --user-name $USER
aws iam create-access-key --user-name $USER > /tmp/newkey.json   # use a tmpfs path
```

1. Write the new key into `$VALUES/velero-s3.env` (the `cloud` key in AWS credentials-file format) or into `$VALUES/satark-pg-backup-s3.env`. Then seal it, open a PR and merge.
2. Restart the consumer: `kubectl -n velero rollout restart deploy/velero`. For CNPG, the next WAL archive picks the key up automatically.
3. Verify that the new key is in use:
   ```bash
   velero backup create rotate-check-$(date +%s) --include-namespaces satark --wait
   kubectl -n satark-data get cluster satark-pg -o jsonpath='{.status.conditions[?(@.type=="ContinuousArchiving")].status}'   # True
   aws iam get-access-key-last-used --access-key-id <NEW_KEY_ID>
   ```
4. Revoke the old key:
   ```bash
   aws iam update-access-key --user-name $USER --access-key-id <OLD_KEY_ID> --status Inactive
   # wait 24h with no errors, then:
   aws iam delete-access-key --user-name $USER --access-key-id <OLD_KEY_ID>
   shred -u /tmp/newkey.json
   ```

If the keys are managed by Terraform (`create_access_keys = true`), run `terraform apply -replace='module.backup_iam.aws_iam_access_key.backup["velero"]'` instead of the CLI. Be aware that this revokes the old key at apply time.

## 5. Sealed Secrets controller key

The controller adds a new sealing key every 30 days (`keyrenewperiod: 720h`) and keeps the old ones so existing SealedSecrets still decrypt.

1. Back up all keys and store the backup offline, encrypted with age:
   ```bash
   kubectl -n kube-system get secret -l sealedsecrets.bitnami.com/sealed-secrets-key -o yaml \
     | age -r <offline-recipient> > sealed-secrets-keys-$(date +%F).yaml.age
   ```
2. Re-encrypt every SealedSecret with the newest key, so retired keys can be deleted later:
   ```bash
   for f in gitops/envs/$ENV/secrets/*.sealed.yaml; do
     kubeseal --re-encrypt < "$f" > "$f.tmp" && mv "$f.tmp" "$f"
   done
   ```
   Open a PR and merge it.
3. To force a rotation after a suspected compromise of the controller key: label the current key `sealedsecrets.bitnami.com/sealed-secrets-key=compromised`, restart the controller, re-seal **every** secret from `$VALUES` with `scripts/seal-all-secrets.sh $ENV $VALUES`, and rotate every underlying value (sections 1 to 4). The old ciphertext is still in git history.

## 6. Confirmed leak

1. **Revoke the leaked credential first**, then follow the matching section above to issue a new one.
2. Purge the leaked value from the default branch. Treat it as compromised forever, even after a history rewrite.
3. Check the access logs. For Kubernetes, query `{job="kubernetes-audit", resource="secrets"}` in Loki. For AWS, use CloudTrail and `get-access-key-last-used`.
4. Write a postmortem in `docs/game-days/` or in the incident tracker.

## 7. Verification

- [ ] `kubectl -n <ns> get sealedsecret <name>` shows `Synced=True`, and the Secret's `metadata.resourceVersion` changed.
- [ ] Every consumer in the inventory is restarted, Ready, and passes `/readyz`.
- [ ] The old credential is revoked and fails if tried: provider console, `aws iam list-access-keys` shows only the new key, or `psql` with the old password is refused.
- [ ] `gitleaks git --config .gitleaks.toml` is clean and `scripts/check-no-plain-secrets.sh` passes.
- [ ] Access Analyzer reports no unused-key finding for the rotated user (`scripts/access-analyzer-report.sh $ENV`).
- [ ] The rotation date is recorded in the table below.

## Rotation log

| Date | Env | Secret | Reason | By |
|---|---|---|---|---|
| | | | | |
