output "vpc_id" {
  description = "Edge VPC ID."
  value       = aws_vpc.this.id
}

output "instance_id" {
  description = "Edge instance ID (use with aws ssm start-session)."
  value       = aws_instance.edge.id
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
