output "policy_arn" {
  description = "ARN of the DNS-01 policy."
  value       = aws_iam_policy.dns01.arn
}

output "user_name" {
  description = "IAM user for cert-manager (create its access key manually)."
  value       = var.create_user ? aws_iam_user.cert_manager[0].name : null
}
