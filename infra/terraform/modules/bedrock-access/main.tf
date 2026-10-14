terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

data "aws_partition" "current" {}
data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  tags = merge(var.tags, {
    Project = var.project
    Owner   = var.owner
  })

  model_arns = [
    for id in var.model_ids :
    "arn:${data.aws_partition.current.partition}:bedrock:${data.aws_region.current.name}::foundation-model/${id}"
  ]
}

# Least-privilege: invoke only the allow-listed foundation models, nothing else.
data "aws_iam_policy_document" "invoke" {
  statement {
    sid    = "InvokeAllowListedModels"
    effect = "Allow"
    actions = [
      "bedrock:InvokeModel",
      "bedrock:InvokeModelWithResponseStream",
      "bedrock:Converse",
      "bedrock:ConverseStream",
    ]
    resources = local.model_arns
  }
}

resource "aws_iam_policy" "invoke" {
  name        = "${var.name}-bedrock-invoke"
  description = "Invoke allow-listed Bedrock models for ${var.name}"
  policy      = data.aws_iam_policy_document.invoke.json
  tags        = local.tags
}

data "aws_iam_policy_document" "trust" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole", "sts:TagSession"]
    principals {
      type        = "AWS"
      identifiers = var.trusted_principal_arns
    }
    dynamic "condition" {
      for_each = var.external_id == null ? [] : [var.external_id]
      content {
        test     = "StringEquals"
        variable = "sts:ExternalId"
        values   = [condition.value]
      }
    }
  }
}

resource "aws_iam_role" "this" {
  name                 = "${var.name}-bedrock"
  description          = "Role assumed by Satark workloads to call Amazon Bedrock"
  assume_role_policy   = data.aws_iam_policy_document.trust.json
  max_session_duration = var.max_session_duration
  tags                 = local.tags
}

resource "aws_iam_role_policy_attachment" "invoke" {
  role       = aws_iam_role.this.name
  policy_arn = aws_iam_policy.invoke.arn
}

# Optional: model invocation logging (metadata only by default; no prompt bodies).
resource "aws_cloudwatch_log_group" "invocations" {
  count             = var.enable_invocation_logging ? 1 : 0
  name              = "/satark/bedrock/${var.name}"
  retention_in_days = var.log_retention_days
  kms_key_id        = var.log_kms_key_arn
  tags              = local.tags
}

data "aws_iam_policy_document" "logging_trust" {
  count = var.enable_invocation_logging ? 1 : 0
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["bedrock.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

resource "aws_iam_role" "logging" {
  count              = var.enable_invocation_logging ? 1 : 0
  name               = "${var.name}-bedrock-logging"
  assume_role_policy = data.aws_iam_policy_document.logging_trust[0].json
  tags               = local.tags
}

data "aws_iam_policy_document" "logging" {
  count = var.enable_invocation_logging ? 1 : 0
  statement {
    effect    = "Allow"
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = ["${aws_cloudwatch_log_group.invocations[0].arn}:*"]
  }
}

resource "aws_iam_role_policy" "logging" {
  count  = var.enable_invocation_logging ? 1 : 0
  name   = "write-invocation-logs"
  role   = aws_iam_role.logging[0].id
  policy = data.aws_iam_policy_document.logging[0].json
}

resource "aws_bedrock_model_invocation_logging_configuration" "this" {
  count = var.enable_invocation_logging ? 1 : 0
  logging_config {
    text_data_delivery_enabled      = false
    image_data_delivery_enabled     = false
    embedding_data_delivery_enabled = false
    cloudwatch_config {
      log_group_name = aws_cloudwatch_log_group.invocations[0].name
      role_arn       = aws_iam_role.logging[0].arn
    }
  }
}
