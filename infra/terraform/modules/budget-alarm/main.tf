# Monthly cost budget with email alerts. Apply this module before any other
# resource so a runaway bill is noticed early (task 5.2).

terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

resource "aws_budgets_budget" "monthly" {
  name         = "${var.name}-monthly-cost"
  budget_type  = "COST"
  limit_amount = format("%.2f", var.monthly_limit_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  cost_types {
    include_credit = false
    include_refund = false
  }

  dynamic "notification" {
    for_each = var.actual_thresholds_percent
    content {
      comparison_operator        = "GREATER_THAN"
      notification_type          = "ACTUAL"
      threshold                  = notification.value
      threshold_type             = "PERCENTAGE"
      subscriber_email_addresses = var.alert_emails
    }
  }

  dynamic "notification" {
    for_each = var.forecast_threshold_percent == null ? [] : [var.forecast_threshold_percent]
    content {
      comparison_operator        = "GREATER_THAN"
      notification_type          = "FORECASTED"
      threshold                  = notification.value
      threshold_type             = "PERCENTAGE"
      subscriber_email_addresses = var.alert_emails
    }
  }

  tags = var.tags
}
