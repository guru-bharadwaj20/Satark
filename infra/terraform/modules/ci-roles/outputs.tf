output "plan_role_arn" {
  description = "Role for terraform plan (store as the AWS_PLAN_ROLE_ARN repository variable)."
  value       = module.plan.role_arn
}

output "apply_role_arn" {
  description = "Role for terraform apply (store as the AWS_APPLY_ROLE_ARN environment variable)."
  value       = module.apply.role_arn
}
