# Bucket-scoped IAM users for Velero and CloudNativePG backups.
#
# Each consumer gets its own IAM user whose only permissions are object
# read/write/list on its own bucket (and prefix). No wildcard resources, no
# bucket administration, no delete-bucket or policy changes. Access is
# further restricted to TLS and, optionally, to the cluster's egress CIDRs.

locals {
  tags = merge(var.tags, {
    Project = var.project
    Owner   = var.owner
    Module  = "backup-iam"
  })

  users = {
    velero = {
      bucket_arn = var.velero_bucket_arn
      prefix     = var.velero_prefix
      # Velero also snapshots EBS only when configured; on-prem uses restic/kopia.
    }
    cnpg = {
      bucket_arn = var.cnpg_bucket_arn
      prefix     = var.cnpg_prefix
    }
  }
}

resource "aws_iam_user" "backup" {
  for_each = local.users

  name          = "${var.project}-${var.environment}-${each.key}-backup"
  path          = "/${var.project}/backup/"
  force_destroy = false
  tags          = merge(local.tags, { Consumer = each.key })
}

data "aws_iam_policy_document" "backup" {
  for_each = local.users

  statement {
    sid       = "ListOwnPrefix"
    actions   = ["s3:ListBucket", "s3:GetBucketLocation"]
    resources = [each.value.bucket_arn]

    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["${each.value.prefix}", "${each.value.prefix}*"]
    }
  }

  statement {
    sid = "ObjectReadWrite"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:AbortMultipartUpload",
      "s3:ListMultipartUploadParts",
    ]
    resources = ["${each.value.bucket_arn}/${each.value.prefix}*"]
  }

  statement {
    sid       = "DenyInsecureTransport"
    effect    = "Deny"
    actions   = ["s3:*"]
    resources = [each.value.bucket_arn, "${each.value.bucket_arn}/*"]

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }

  dynamic "statement" {
    for_each = length(var.allowed_source_cidrs) > 0 ? [1] : []
    content {
      sid       = "DenyOutsideClusterEgress"
      effect    = "Deny"
      actions   = ["s3:*"]
      resources = [each.value.bucket_arn, "${each.value.bucket_arn}/*"]

      condition {
        test     = "NotIpAddress"
        variable = "aws:SourceIp"
        values   = var.allowed_source_cidrs
      }
    }
  }

  dynamic "statement" {
    for_each = var.kms_key_arn != null ? [1] : []
    content {
      sid       = "UseBucketKmsKey"
      actions   = ["kms:Encrypt", "kms:Decrypt", "kms:GenerateDataKey", "kms:DescribeKey"]
      resources = [var.kms_key_arn]
    }
  }
}

resource "aws_iam_user_policy" "backup" {
  for_each = local.users

  name   = "${each.key}-bucket-scoped"
  user   = aws_iam_user.backup[each.key].name
  policy = data.aws_iam_policy_document.backup[each.key].json
}

resource "aws_iam_access_key" "backup" {
  for_each = var.create_access_keys ? local.users : {}

  user = aws_iam_user.backup[each.key].name
}
