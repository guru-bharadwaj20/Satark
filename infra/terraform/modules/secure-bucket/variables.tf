variable "name" {
  description = "Globally unique bucket name."
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$", var.name))
    error_message = "Bucket name must be 3-63 characters of lowercase letters, digits, dots and hyphens."
  }
}

variable "versioning" {
  description = "Enable object versioning."
  type        = bool
  default     = true
}

variable "kms_key_arn" {
  description = "KMS key ARN for SSE-KMS. When null, SSE-S3 (AES256) is used."
  type        = string
  default     = null
}

variable "transition_to_ia_days" {
  description = "Move current objects to STANDARD_IA after this many days. Null disables the transition."
  type        = number
  default     = null
}

variable "expiration_days" {
  description = "Expire current objects after this many days. Null keeps them forever."
  type        = number
  default     = null
}

variable "noncurrent_version_expiration_days" {
  description = "Delete noncurrent object versions after this many days."
  type        = number
  default     = 30
}

variable "force_destroy" {
  description = "Allow terraform destroy to delete a non-empty bucket. Keep false outside dev."
  type        = bool
  default     = false
}

variable "extra_policy_json" {
  description = "Additional bucket policy statements (JSON) merged with the TLS-only statements."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags applied to every resource. Must include Project, Environment, Owner and ManagedBy."
  type        = map(string)
}
