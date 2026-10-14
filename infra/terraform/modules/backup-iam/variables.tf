variable "project" {
  description = "Project name, used in resource names and the Project tag."
  type        = string
  default     = "satark"
}

variable "owner" {
  description = "Owner tag value."
  type        = string
}

variable "environment" {
  description = "Environment name (dev or prod)."
  type        = string

  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod."
  }
}

variable "velero_bucket_arn" {
  description = "ARN of the Velero backup bucket."
  type        = string
}

variable "velero_prefix" {
  description = "Object prefix Velero may use inside its bucket (with trailing slash, or empty for the whole bucket)."
  type        = string
  default     = ""
}

variable "cnpg_bucket_arn" {
  description = "ARN of the Postgres WAL/base backup bucket used by CloudNativePG."
  type        = string
}

variable "cnpg_prefix" {
  description = "Object prefix CNPG may use inside its bucket."
  type        = string
  default     = "satark-pg/"
}

variable "kms_key_arn" {
  description = "KMS key ARN used for bucket encryption, or null for SSE-S3."
  type        = string
  default     = null
}

variable "allowed_source_cidrs" {
  description = "If set, deny bucket access from any other source IP (the cluster's NAT/egress addresses)."
  type        = list(string)
  default     = []
}

variable "create_access_keys" {
  description = "Create static access keys. Keys land in Terraform state, so state must be encrypted; rotate per SOP-03."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Extra tags."
  type        = map(string)
  default     = {}
}
