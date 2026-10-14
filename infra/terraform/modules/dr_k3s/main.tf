terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"]
  }
}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

resource "aws_security_group" "dr" {
  name_prefix = "${var.name}-"
  description = "Temporary DR k3s node"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "HTTPS from anywhere (public API during DR)"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTP for ACME challenges"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "Kubernetes API from operators only"
    from_port   = 6443
    to_port     = 6443
    protocol    = "tcp"
    cidr_blocks = var.admin_cidrs
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.tags
}

# Instance role: read the backup bucket so Velero can restore. No write
# access, so a DR drill can never damage production backups.
data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

data "aws_iam_policy_document" "backup_read" {
  statement {
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = ["arn:aws:s3:::${var.backup_bucket}"]
  }
  statement {
    actions   = ["s3:GetObject", "s3:GetObjectVersion"]
    resources = ["arn:aws:s3:::${var.backup_bucket}/*"]
  }
}

resource "aws_iam_role" "dr" {
  name_prefix        = "${var.name}-"
  assume_role_policy = data.aws_iam_policy_document.assume.json
  tags               = local.tags
}

resource "aws_iam_role_policy" "backup_read" {
  role   = aws_iam_role.dr.id
  policy = data.aws_iam_policy_document.backup_read.json
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.dr.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "dr" {
  name_prefix = "${var.name}-"
  role        = aws_iam_role.dr.name
}

resource "aws_instance" "dr" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  subnet_id              = data.aws_subnets.default.ids[0]
  vpc_security_group_ids = [aws_security_group.dr.id]
  iam_instance_profile   = aws_iam_instance_profile.dr.name
  key_name               = var.key_name

  metadata_options {
    http_tokens = "required"
  }

  root_block_device {
    volume_size = var.disk_gb
    volume_type = "gp3"
    encrypted   = true
  }

  user_data = templatefile("${path.module}/cloud-init.sh.tftpl", {
    k3s_version = var.k3s_version
  })

  tags = merge(local.tags, { Name = var.name })
}

resource "aws_eip" "dr" {
  instance = aws_instance.dr.id
  domain   = "vpc"
  tags     = local.tags
}

locals {
  tags = merge(var.tags, {
    Project   = "satark"
    Purpose   = "disaster-recovery"
    Temporary = "true"
  })
}
