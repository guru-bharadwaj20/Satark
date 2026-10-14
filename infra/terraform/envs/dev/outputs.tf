output "edge_public_ip" {
  description = "Edge Elastic IP (WireGuard endpoint)."
  value       = module.edge.public_ip
}

output "edge_instance_id" {
  description = "Edge instance ID for SSM sessions."
  value       = module.edge.instance_id
}

output "zone_id" {
  description = "Route 53 hosted zone ID."
  value       = module.dns.zone_id
}

output "name_servers" {
  description = "Name servers to delegate to."
  value       = module.dns.name_servers
}

output "buckets" {
  description = "Application bucket names."
  value = {
    velero      = module.storage.velero_bucket
    pg_wal      = module.storage.pg_wal_bucket
    screenshots = module.storage.screenshots_bucket
  }
}

output "bucket_policy_arns" {
  description = "IAM policies for bucket access."
  value       = module.storage.policy_arns
}

output "cert_manager_user" {
  description = "IAM user for cert-manager DNS-01."
  value       = module.cert_manager_dns01.user_name
}
