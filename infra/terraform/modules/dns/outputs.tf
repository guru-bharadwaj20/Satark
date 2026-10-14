output "zone_id" {
  description = "Hosted zone ID."
  value       = local.zone_id
}

output "name_servers" {
  description = "Name servers to set at the registrar (only when the zone is created here)."
  value       = var.create_zone ? aws_route53_zone.this[0].name_servers : []
}

output "edge_fqdns" {
  description = "Hostnames pointing to the edge."
  value       = values(local.edge_names)
}
