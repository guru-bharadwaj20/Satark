# bedrock-access

Least-privilege access to Amazon Bedrock for `text-analyzer` and `opsbot`.

- An IAM role that only the listed principals can assume (no wildcards).
- An invoke policy scoped to an allow-list of foundation model ARNs.
- Optional invocation logging to CloudWatch with prompt/response bodies **disabled**,
  so redacted user text never lands in AWS logs.
- Every resource is tagged with `Project` and `Owner`.

Model access itself must be enabled once per account and region in the Bedrock console.

```hcl
module "bedrock_text_analyzer" {
  source                 = "../../modules/bedrock-access"
  name                   = "satark-dev-text-analyzer"
  owner                  = "guru-bharadwaj20"
  model_ids              = ["anthropic.claude-3-haiku-20240307-v1:0"]
  trusted_principal_arns = [module.workload_identity.role_arn]
}
```

Set `SATARK_LLM_PROVIDER=bedrock`, `SATARK_BEDROCK_MODEL` and `AWS_REGION` in the workload,
with credentials for a session on `role_arn`.

| Output | Description |
|---|---|
| `role_arn` | Role to assume |
| `policy_arn` | Invoke policy |
| `model_arns` | Allowed model ARNs |
| `invocation_log_group` | Log group, or null |
