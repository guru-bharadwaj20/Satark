variable "name_prefix" {
  description = "Prefix for bucket and policy names."
  type        = string
  default     = "satark"
}

variable "environment" {
  description = "Environment name (dev or prod)."
  type        = string
}

variable "velero_retention_days" {
  description = "Days to keep Velero backups."
  type        = number
  default     = 30
}

variable "wal_retention_days" {
  description = "Days to keep WAL and base backups. Must exceed the CloudNativePG retention policy."
  type        = number
  default     = 14
}

variable "screenshot_retention_days" {
  description = "Days to keep sandbox screenshots."
  type        = number
  default     = 7
}

variable "force_destroy" {
  description = "Allow destroying non-empty buckets (dev only)."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Tags applied to every resource."
  type        = map(string)
}
