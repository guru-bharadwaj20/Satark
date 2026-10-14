output "role_arn" {
  description = "ARN to put in aws-actions/configure-aws-credentials role-to-assume."
  value       = aws_iam_role.this.arn
}

output "role_name" {
  description = "Role name."
  value       = aws_iam_role.this.name
}

output "oidc_provider_arn" {
  description = "ARN of the GitHub OIDC provider used by the role."
  value       = local.provider_arn
}
