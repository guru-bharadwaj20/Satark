# Edge security group (task 5.9). Inbound is limited to:
#   80/tcp     HTTP (redirect to HTTPS, ACME HTTP-01 fallback)
#   443/tcp    HTTPS, passed through by SNI to the cluster
#   51820/udp  WireGuard from the on-prem worker
# There is deliberately no port 22: shell access goes through SSM Session
# Manager (task 5.11).

locals {
  ingress_rules = {
    http = {
      port        = 80
      protocol    = "tcp"
      description = "HTTP"
      cidrs       = var.public_ingress_cidrs
    }
    https = {
      port        = 443
      protocol    = "tcp"
      description = "HTTPS (SNI passthrough)"
      cidrs       = var.public_ingress_cidrs
    }
    wireguard = {
      port        = var.wireguard_port
      protocol    = "udp"
      description = "WireGuard tunnel from on-prem"
      cidrs       = var.wireguard_peer_cidrs
    }
  }

  # Flatten to one rule per CIDR, as the per-rule resources expect.
  ingress = merge([
    for name, r in local.ingress_rules : {
      for cidr in r.cidrs : "${name}-${cidr}" => merge(r, { cidr = cidr })
    }
  ]...)
}

resource "aws_security_group" "edge" {
  name        = "${var.name}-sg"
  description = "Satark edge: 80, 443 and WireGuard only"
  vpc_id      = aws_vpc.this.id

  tags = merge(var.tags, { Name = "${var.name}-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_vpc_security_group_ingress_rule" "edge" {
  for_each = local.ingress

  security_group_id = aws_security_group.edge.id
  description       = each.value.description
  ip_protocol       = each.value.protocol
  from_port         = each.value.port
  to_port           = each.value.port
  cidr_ipv4         = each.value.cidr

  tags = var.tags
}

resource "aws_vpc_security_group_egress_rule" "all" {
  security_group_id = aws_security_group.edge.id
  description       = "Outbound for SSM, package updates, ACME and the tunnel"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"

  tags = var.tags
}
