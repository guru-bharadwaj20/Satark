# IAM Access Analyzer

Analyzers are created by `infra/terraform/modules/access-analyzer`:

- `satark-<env>-external`: resources (S3 buckets, KMS keys, IAM roles) that a principal outside the account can reach.
- `satark-<env>-unused`: IAM users and roles with access keys, passwords or permissions unused for 90 days.

## Review procedure

1. Export the active findings:
   `scripts/access-analyzer-report.sh dev > docs/compliance/access-analyzer-<date>.md`
2. Handle each finding in one of three ways:
   - **Fix in Terraform.** This is the default. Tighten the bucket policy or trust policy, or remove the unused permission or key, then run `terraform plan` and `terraform apply` through CI.
   - **Accept.** Add an archive rule (the `archive_rules` variable) with a comment explaining why, for example the public `pub-cert.pem` bucket object if one is ever published.
   - **Rotate.** For unused or old keys, follow SOP-03.
3. Re-run the export and confirm that no ACTIVE findings remain.

## Expected findings and their fixes

| Finding | Likely cause | Fix |
|---|---|---|
| S3 bucket shared outside account | Bucket policy with `Principal: *` and no condition | The `secure-bucket` module's TLS-only and public access block, and the `aws:SourceIp` condition in `backup-iam` |
| Unused access key on backup user | Velero or CNPG still using an older key after rotation | Delete the old key (SOP-03 step "revoke") |
| Unused permission `s3:DeleteObject` on CNPG user | Retention is handled by bucket lifecycle | Remove it from the `backup-iam` CNPG statement if it stays unused after 90 days |
| Role trusted by GitHub OIDC with broad `sub` | Trust condition `repo:*` | Restrict `token.actions.githubusercontent.com:sub` to `repo:guru-bharadwaj20/satark:*` |

## Review log

| Date | Env | Active before | Fixed | Archived | Active after |
|---|---|---|---|---|---|
| _pending first run in the AWS account_ | | | | | |
