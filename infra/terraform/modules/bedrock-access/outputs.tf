output "role_arn" {
  description = "ARN of the role workloads assume to call Bedrock."
  value       = aws_iam_role.this.arn
}

output "policy_arn" {
  description = "ARN of the least-privilege invoke policy."
  value       = aws_iam_policy.invoke.arn
}

output "model_arns" {
  description = "Foundation model ARNs the role may invoke."
  value       = local.model_arns
}

output "invocation_log_group" {
  description = "CloudWatch log group for invocation metadata (null when disabled)."
  value       = try(aws_cloudwatch_log_group.invocations[0].name, null)
}
