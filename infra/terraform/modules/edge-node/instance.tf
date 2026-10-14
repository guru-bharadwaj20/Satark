# Edge EC2 instance (task 5.10): Graviton (arm64), IMDSv2 required, encrypted
# gp3 root volume, and an Elastic IP so DNS records stay stable across
# instance replacement.

data "aws_ssm_parameter" "ubuntu_arm64" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/arm64/hvm/ebs-gp3/ami-id"
}

resource "aws_instance" "edge" {
  ami                    = coalesce(var.ami_id, data.aws_ssm_parameter.ubuntu_arm64.value)
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.edge.id]
  monitoring             = var.detailed_monitoring
  ebs_optimized          = true
  # The edge forwards tunnel traffic for other addresses.
  source_dest_check = false

  user_data_base64            = var.user_data == null ? null : base64encode(var.user_data)
  user_data_replace_on_change = false

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
    instance_metadata_tags      = "enabled"
  }

  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_gb
    encrypted             = true
    kms_key_id            = var.ebs_kms_key_arn
    delete_on_termination = true
    tags                  = merge(var.tags, { Name = "${var.name}-root" })
  }

  credit_specification {
    cpu_credits = "standard"
  }

  tags = merge(var.tags, { Name = var.name, Role = "edge" })

  lifecycle {
    # AMI updates happen through planned replacement, not on every new release.
    ignore_changes = [ami]

    precondition {
      condition     = can(regex("^(t4g|c7g|c6g|m7g|m6g|c8g|m8g)\\.", var.instance_type))
      error_message = "instance_type must be a Graviton (arm64) type."
    }
  }
}

resource "aws_eip" "edge" {
  domain = "vpc"
  tags   = merge(var.tags, { Name = "${var.name}-eip" })

  depends_on = [aws_internet_gateway.this]
}

resource "aws_eip_association" "edge" {
  instance_id   = aws_instance.edge.id
  allocation_id = aws_eip.edge.id
}
