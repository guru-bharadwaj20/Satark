output "velero_bucket" {
  description = "Velero backup bucket name."
  value       = module.velero.id
}

output "pg_wal_bucket" {
  description = "Postgres WAL archive bucket name."
  value       = module.pg_wal.id
}

output "screenshots_bucket" {
  description = "Sandbox screenshot bucket name."
  value       = module.screenshots.id
}

output "policy_arns" {
  description = "IAM policy ARNs granting access to each bucket."
  value = {
    velero      = aws_iam_policy.velero.arn
    pg_wal      = aws_iam_policy.pg_wal.arn
    screenshots = aws_iam_policy.screenshots.arn
  }
}
