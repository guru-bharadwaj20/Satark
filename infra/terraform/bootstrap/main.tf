# Bootstrap stack.
#
# Creates the S3 bucket that holds Terraform state for every other stack, and
# the cost budget so it exists before anything else is billed.
#
# This stack uses LOCAL state on purpose: it creates the bucket a remote
# backend would need. Keep the resulting terraform.tfstate out of git (it is
# ignored) and back it up somewhere safe, or migrate it into the bucket after
# the first apply with `terraform init -migrate-state` and a backend block.
#
# Other stacks lock state with S3 native locking (`use_lockfile = true`,
# Terraform >= 1.10), so no DynamoDB table is needed.

terraform {
  required_version = ">= 1.10.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = local.tags
  }
}

data "aws_caller_identity" "current" {}

locals {
  tags = {
    Project     = "satark"
    Environment = "shared"
    Owner       = var.owner
    ManagedBy   = "terraform"
  }
  state_bucket = "${var.name_prefix}-tfstate-${data.aws_caller_identity.current.account_id}"
}

module "budget" {
  source = "../modules/budget-alarm"

  name              = var.name_prefix
  monthly_limit_usd = var.monthly_budget_usd
  alert_emails      = var.alert_emails
  tags              = local.tags
}

# CI identities live here, not in envs/*, so the apply role can never modify
# its own permissions.
module "ci_roles" {
  source = "../modules/ci-roles"

  name_prefix  = var.name_prefix
  repository   = var.github_repository
  state_bucket = aws_s3_bucket.state.bucket
  tags         = local.tags
}

resource "aws_s3_bucket" "state" {
  bucket = local.state_bucket

  # Losing state is far worse than an orphaned bucket.
  lifecycle {
    prevent_destroy = true
  }

  tags = local.tags
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket                  = aws_s3_bucket.state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "state" {
  bucket = aws_s3_bucket.state.id

  rule {
    id     = "expire-old-state-versions"
    status = "Enabled"
    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.noncurrent_version_days
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

data "aws_iam_policy_document" "state" {
  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [aws_s3_bucket.state.arn, "${aws_s3_bucket.state.arn}/*"]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "state" {
  bucket = aws_s3_bucket.state.id
  policy = data.aws_iam_policy_document.state.json

  depends_on = [aws_s3_bucket_public_access_block.state]
}
