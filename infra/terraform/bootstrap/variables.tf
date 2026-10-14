variable "region" {
  description = "AWS region for the state bucket."
  type        = string
  default     = "ap-south-1"
}

variable "name_prefix" {
  description = "Prefix for resource names."
  type        = string
  default     = "satark"
}

variable "owner" {
  description = "Value of the Owner tag."
  type        = string
  default     = "guru-bharadwaj20"
}

variable "monthly_budget_usd" {
  description = "Monthly cost budget in USD."
  type        = number
  default     = 10
}

variable "alert_emails" {
  description = "Email addresses for budget alerts."
  type        = list(string)
}

variable "noncurrent_version_days" {
  description = "Days to keep old versions of state files."
  type        = number
  default     = 90
}
