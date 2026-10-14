variable "role_name" {
  description = "Name of the IAM role."
  type        = string
}

variable "description" {
  description = "Role description."
  type        = string
  default     = "Assumed by GitHub Actions through OIDC"
}

variable "repository" {
  description = "GitHub repository in owner/name form."
  type        = string

  validation {
    condition     = can(regex("^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$", var.repository))
    error_message = "repository must look like owner/name."
  }
}

variable "subject_claims" {
  description = "Allowed `sub` suffixes after repo:<owner>/<name>:, e.g. \"ref:refs/heads/main\", \"pull_request\", \"environment:prod\"."
  type        = list(string)

  validation {
    condition     = length(var.subject_claims) > 0 && alltrue([for s in var.subject_claims : s != "*"])
    error_message = "Provide at least one subject claim, and never a bare \"*\"."
  }
}

variable "create_oidc_provider" {
  description = "Create the GitHub OIDC provider. Only one may exist per account, so set false for the second role and pass oidc_provider_arn."
  type        = bool
  default     = true
}

variable "oidc_provider_arn" {
  description = "Existing GitHub OIDC provider ARN, used when create_oidc_provider is false."
  type        = string
  default     = null
}

variable "managed_policy_arns" {
  description = "Managed policies attached to the role."
  type        = list(string)
  default     = []
}

variable "inline_policy_json" {
  description = "Optional inline policy document."
  type        = string
  default     = null
}

variable "permissions_boundary_arn" {
  description = "Optional permissions boundary."
  type        = string
  default     = null
}

variable "max_session_duration" {
  description = "Maximum session duration in seconds."
  type        = number
  default     = 3600
}

variable "tags" {
  description = "Tags applied to every resource."
  type        = map(string)
}
