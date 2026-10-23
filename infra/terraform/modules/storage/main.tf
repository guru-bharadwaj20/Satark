# Application buckets:
#   - velero:      cluster backups (Velero)
#   - pg-wal:      CloudNativePG base backups and WAL archive for satark-pg
#   - screenshots: sandbox-browser page screenshots (short retention, may hold PII)
#
# Each bucket also gets a least-privilege IAM policy. The on-prem cluster has
# no IRSA, so the policies are attached to whichever identity the cluster
# workloads use (for example an IAM user whose keys live in a sealed secret).

terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

data "aws_caller_identity" "current" {}

locals {
  suffix = "${var.environment}-${data.aws_caller_identity.current.account_id}"
}

module "velero" {
  source = "../secure-bucket"

  name                               = "${var.name_prefix}-velero-${local.suffix}"
  transition_to_ia_days              = 30
  expiration_days                    = var.velero_retention_days
  noncurrent_version_expiration_days = 7
  force_destroy                      = var.force_destroy
  tags                               = merge(var.tags, { Purpose = "velero-backups" })
}

module "pg_wal" {
  source = "../secure-bucket"

  name                               = "${var.name_prefix}-pg-wal-${local.suffix}"
  expiration_days                    = var.wal_retention_days
  noncurrent_version_expiration_days = 7
  force_destroy                      = var.force_destroy
  tags                               = merge(var.tags, { Purpose = "postgres-wal" })
}

module "screenshots" {
  source = "../secure-bucket"

  name = "${var.name_prefix}-screenshots-${local.suffix}"
  # Screenshots can contain personal data from phishing pages: keep them briefly.
  versioning      = false
  expiration_days = var.screenshot_retention_days
  force_destroy   = var.force_destroy
  tags            = merge(var.tags, { Purpose = "sandbox-screenshots" })
}

data "aws_iam_policy_document" "velero" {
  statement {
    sid       = "Objects"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"]
    resources = ["${module.velero.arn}/*"]
  }
  statement {
    sid       = "Bucket"
    actions   = ["s3:ListBucket"]
    resources = [module.velero.arn]
  }
}

data "aws_iam_policy_document" "pg_wal" {
  statement {
    sid       = "Objects"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["${module.pg_wal.arn}/*"]
  }
  statement {
    sid       = "Bucket"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [module.pg_wal.arn]
  }
}

data "aws_iam_policy_document" "screenshots" {
  statement {
    sid       = "Objects"
    actions   = ["s3:GetObject", "s3:PutObject"]
    resources = ["${module.screenshots.arn}/*"]
  }
}

resource "aws_iam_policy" "velero" {
  name        = "${var.name_prefix}-${var.environment}-velero-s3"
  description = "Velero access to its backup bucket."
  policy      = data.aws_iam_policy_document.velero.json
  tags        = var.tags
}

resource "aws_iam_policy" "pg_wal" {
  name        = "${var.name_prefix}-${var.environment}-pg-wal-s3"
  description = "CloudNativePG access to the WAL archive bucket."
  policy      = data.aws_iam_policy_document.pg_wal.json
  tags        = var.tags
}

resource "aws_iam_policy" "screenshots" {
  name        = "${var.name_prefix}-${var.environment}-screenshots-s3"
  description = "sandbox-browser access to the screenshot bucket."
  policy      = data.aws_iam_policy_document.screenshots.json
  tags        = var.tags
}
