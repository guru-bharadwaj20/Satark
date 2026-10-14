# IAM Access Analyzer (task 9.17): an external-access analyzer flags any
# bucket, key or role reachable from outside the account, and an
# unused-access analyzer flags IAM users/roles with stale keys or
# permissions (e.g. the backup users from the backup-iam module).

terraform {
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.80"
    }
  }
}

variable "project" {
  type    = string
  default = "satark"
}

variable "owner" {
  type = string
}

variable "environment" {
  type = string
}

variable "unused_access_age_days" {
  description = "Report credentials and permissions unused for this many days."
  type        = number
  default     = 90
}

variable "archive_rules" {
  description = "Findings to auto-archive (reviewed and accepted). Map of rule name to list of {criteria, eq} filters."
  type = map(list(object({
    criteria = string
    eq       = list(string)
  })))
  default = {}
}

locals {
  tags = {
    Project     = var.project
    Owner       = var.owner
    Environment = var.environment
    Module      = "access-analyzer"
  }
}

resource "aws_accessanalyzer_analyzer" "external" {
  analyzer_name = "${var.project}-${var.environment}-external"
  type          = "ACCOUNT"
  tags          = local.tags
}

resource "aws_accessanalyzer_analyzer" "unused" {
  analyzer_name = "${var.project}-${var.environment}-unused"
  type          = "ACCOUNT_UNUSED_ACCESS"
  tags          = local.tags

  configuration {
    unused_access {
      unused_access_age = var.unused_access_age_days
    }
  }
}

resource "aws_accessanalyzer_archive_rule" "accepted" {
  for_each = var.archive_rules

  analyzer_name = aws_accessanalyzer_analyzer.external.analyzer_name
  rule_name     = each.key

  dynamic "filter" {
    for_each = each.value
    content {
      criteria = filter.value.criteria
      eq       = filter.value.eq
    }
  }
}

output "external_analyzer_arn" {
  value = aws_accessanalyzer_analyzer.external.arn
}

output "unused_analyzer_arn" {
  value = aws_accessanalyzer_analyzer.unused.arn
}
