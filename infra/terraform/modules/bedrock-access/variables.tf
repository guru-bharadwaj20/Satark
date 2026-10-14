variable "name" {
  description = "Name prefix for created resources, e.g. satark-dev-text-analyzer."
  type        = string
}

variable "project" {
  description = "Value of the mandatory Project tag."
  type        = string
  default     = "satark"
}

variable "owner" {
  description = "Value of the mandatory Owner tag."
  type        = string
}

variable "tags" {
  description = "Extra tags merged into every resource."
  type        = map(string)
  default     = {}
}

variable "model_ids" {
  description = "Bedrock foundation model IDs the role may invoke (allow-list)."
  type        = list(string)
  default     = ["anthropic.claude-3-haiku-20240307-v1:0"]

  validation {
    condition     = length(var.model_ids) > 0 && alltrue([for m in var.model_ids : !strcontains(m, "*")])
    error_message = "Provide at least one model ID; wildcards are not allowed."
  }
}

variable "trusted_principal_arns" {
  description = "IAM principals (e.g. the on-prem workload identity role) allowed to assume the Bedrock role."
  type        = list(string)

  validation {
    condition     = length(var.trusted_principal_arns) > 0 && !contains(var.trusted_principal_arns, "*")
    error_message = "At least one explicit principal ARN is required; \"*\" is not allowed."
  }
}

variable "external_id" {
  description = "Optional sts:ExternalId required when assuming the role."
  type        = string
  default     = null
  sensitive   = true
}

variable "max_session_duration" {
  description = "Maximum session duration in seconds."
  type        = number
  default     = 3600
}

variable "enable_invocation_logging" {
  description = "Enable Bedrock invocation logging to CloudWatch (metadata only; prompt and response bodies are never delivered)."
  type        = bool
  default     = true
}

variable "log_retention_days" {
  description = "Retention for the invocation log group."
  type        = number
  default     = 30
}

variable "log_kms_key_arn" {
  description = "Optional KMS key for log group encryption."
  type        = string
  default     = null
}
