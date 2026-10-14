# Fixtures that the custom Satark policies must accept.

resource "aws_s3_bucket" "tagged" {
  bucket = "satark-fixture-tagged"
  tags = {
    Project     = "satark"
    Environment = "test"
    Owner       = "fixture"
    ManagedBy   = "terraform"
  }
}

resource "aws_vpc_security_group_ingress_rule" "https" {
  security_group_id = "sg-123"
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_ingress_rule" "wireguard" {
  security_group_id = "sg-123"
  ip_protocol       = "udp"
  from_port         = 51820
  to_port           = 51820
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group_rule" "ssh_from_lan" {
  type              = "ingress"
  security_group_id = "sg-123"
  from_port         = 22
  to_port           = 22
  protocol          = "tcp"
  cidr_blocks       = ["192.168.56.0/24"]
}
