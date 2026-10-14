output "vpc_id" {
  description = "Edge VPC ID."
  value       = aws_vpc.this.id
}

output "security_group_id" {
  description = "Edge security group ID."
  value       = aws_security_group.edge.id
}

output "public_subnet_id" {
  description = "Public subnet ID."
  value       = aws_subnet.public.id
}
