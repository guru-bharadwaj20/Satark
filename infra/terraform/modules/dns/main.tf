# Route 53 hosted zone and records (task 5.12). Public hostnames point to the
# edge Elastic IP. CAA restricts issuance to Let's Encrypt.

terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

locals {
  zone_id = var.create_zone ? aws_route53_zone.this[0].zone_id : data.aws_route53_zone.existing[0].zone_id

  # Map of record name ("" = apex) to FQDN.
  edge_names = { for n in var.edge_hostnames : n => n == "" ? var.domain : "${n}.${var.domain}" }
}

resource "aws_route53_zone" "this" {
  #checkov:skip=CKV2_AWS_39:Query logging requires a us-east-1 log group; out of scope
  #checkov:skip=CKV2_AWS_38:DNSSEC needs a KMS key in us-east-1 and registrar DS setup; tracked separately
  count = var.create_zone ? 1 : 0

  name          = var.domain
  comment       = "Satark ${var.environment}"
  force_destroy = false

  tags = var.tags
}

data "aws_route53_zone" "existing" {
  count = var.create_zone ? 0 : 1

  name         = var.domain
  private_zone = false
}

resource "aws_route53_record" "edge_a" {
  #checkov:skip=CKV2_AWS_23:Records point at the edge Elastic IP passed in from edge-node
  for_each = local.edge_names

  zone_id = local.zone_id
  name    = each.value
  type    = "A"
  ttl     = var.ttl
  records = [var.edge_ip]
}

resource "aws_route53_record" "caa" {
  zone_id = local.zone_id
  name    = var.domain
  type    = "CAA"
  ttl     = 3600
  records = concat(
    [for ca in var.caa_issuers : "0 issue \"${ca}\""],
    var.caa_iodef_email == null ? [] : ["0 iodef \"mailto:${var.caa_iodef_email}\""],
  )
}

resource "aws_route53_record" "extra" {
  for_each = { for r in var.extra_records : "${r.name}-${r.type}" => r }

  zone_id = local.zone_id
  name    = each.value.name == "" ? var.domain : "${each.value.name}.${var.domain}"
  type    = each.value.type
  ttl     = each.value.ttl
  records = each.value.records
}
