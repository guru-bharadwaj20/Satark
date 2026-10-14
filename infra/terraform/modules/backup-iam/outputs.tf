output "user_arns" {
  description = "ARNs of the backup IAM users, keyed by consumer (velero, cnpg)."
  value       = { for k, u in aws_iam_user.backup : k => u.arn }
}

output "user_names" {
  description = "Names of the backup IAM users, for IAM Access Analyzer and rotation."
  value       = { for k, u in aws_iam_user.backup : k => u.name }
}

output "access_key_ids" {
  description = "Access key IDs, keyed by consumer."
  value       = { for k, key in aws_iam_access_key.backup : k => key.id }
}

output "secret_access_keys" {
  description = "Secret access keys, keyed by consumer. Seal into velero-s3 and satark-pg-backup-s3, never print."
  value       = { for k, key in aws_iam_access_key.backup : k => key.secret }
  sensitive   = true
}
