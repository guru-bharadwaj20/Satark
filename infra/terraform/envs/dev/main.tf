# Composition for this environment (task 5.14): storage buckets, the edge
# node, DNS and the cert-manager DNS-01 identity. The budget and the CI roles
# live in ../../bootstrap.

locals {
  name = "${var.name_prefix}-${var.environment}"

  tags = {
    Project     = "satark"
    Environment = var.environment
    Owner       = var.owner
    ManagedBy   = "terraform"
  }
}

module "storage" {
  source = "../../modules/storage"

  name_prefix               = var.name_prefix
  environment               = var.environment
  velero_retention_days     = var.velero_retention_days
  screenshot_retention_days = var.screenshot_retention_days
  force_destroy             = var.force_destroy_buckets
  tags                      = local.tags
}

module "edge" {
  source = "../../modules/edge-node"

  name                 = "${local.name}-edge"
  vpc_cidr             = var.edge_vpc_cidr
  instance_type        = var.edge_instance_type
  wireguard_peer_cidrs = var.wireguard_peer_cidrs
  alarm_emails         = var.alarm_emails
  tags                 = local.tags
}

module "dns" {
  source = "../../modules/dns"

  domain      = var.domain
  environment = var.environment
  create_zone = var.create_zone
  edge_ip     = module.edge.public_ip
  tags        = local.tags
}

module "cert_manager_dns01" {
  source = "../../modules/cert-manager-dns01"

  name    = local.name
  zone_id = module.dns.zone_id
  tags    = local.tags
}
