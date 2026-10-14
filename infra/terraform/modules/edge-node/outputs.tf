output "vpc_id" {
  description = "Edge VPC ID."
  value       = aws_vpc.this.id
}

output "instance_id" {
  description = "Edge instance ID (use with aws ssm start-session)."
  value       = aws_instance.edge.id
}

output "instance_role_name" {
  description = "Name of the edge instance IAM role."
  value       = aws_iam_role.edge.name
}

output "ssm_session_document" {
  description = "Session Manager preferences document for start-session --document-name."
  value       = aws_ssm_document.session_prefs.name
}

output "metric_namespace" {
  description = "CloudWatch namespace the edge agent publishes to."
  value       = local.metric_namespace
}

output "alerts_topic_arn" {
  description = "SNS topic for edge alarms."
  value       = aws_sns_topic.alerts.arn
}

output "public_ip" {
  description = "Elastic IP of the edge node."
  value       = aws_eip.edge.public_ip
}

output "security_group_id" {
  description = "Edge security group ID."
  value       = aws_security_group.edge.id
}

output "public_subnet_id" {
  description = "Public subnet ID."
  value       = aws_subnet.public.id
}
