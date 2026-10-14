# Fixtures that the custom Satark policies must flag.

resource "aws_s3_bucket" "untagged" {
  bucket = "satark-fixture-untagged"
  tags = {
    Project = "satark"
  }
}

resource "aws_security_group" "inline_ssh" {
  name = "fixture-inline-ssh"
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = {
    Project     = "satark"
    Environment = "test"
    Owner       = "fixture"
    ManagedBy   = "terraform"
  }
}

resource "aws_security_group_rule" "range_ssh" {
  type              = "ingress"
  security_group_id = "sg-123"
  from_port         = 0
  to_port           = 1024
  protocol          = "tcp"
  cidr_blocks       = ["0.0.0.0/0"]
}

resource "aws_vpc_security_group_ingress_rule" "v6_ssh" {
  security_group_id = "sg-123"
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
  cidr_ipv6         = "::/0"
}
