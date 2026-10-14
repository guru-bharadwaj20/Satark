variable "name" {
  description = "Prefix for the budget name."
  type        = string
  default     = "satark"
}

variable "monthly_limit_usd" {
  description = "Monthly cost limit in USD."
  type        = number
  default     = 10

  validation {
    condition     = var.monthly_limit_usd > 0
    error_message = "monthly_limit_usd must be greater than zero."
  }
}

variable "alert_emails" {
  description = "Email addresses notified when a threshold is crossed."
  type        = list(string)

  validation {
    condition     = length(var.alert_emails) > 0
    error_message = "At least one alert email is required."
  }
}

variable "actual_thresholds_percent" {
  description = "Percentages of the limit (actual spend) that trigger an email."
  type        = list(number)
  default     = [50, 80, 100]
}

variable "forecast_threshold_percent" {
  description = "Forecasted spend percentage that triggers an email. Set to null to disable."
  type        = number
  default     = 100
}

variable "tags" {
  description = "Tags applied to every resource. Must include Project, Environment, Owner and ManagedBy."
  type        = map(string)
}
