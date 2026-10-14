output "instance_id" {
  description = "EC2 instance ID (use with aws ssm start-session)"
  value       = aws_instance.dr.id
}

output "public_ip" {
  description = "Elastic IP to point DNS at during failover"
  value       = aws_eip.dr.public_ip
}
