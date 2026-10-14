variable "name_prefix" {
  description = "Prefix for role names. The apply role may only manage IAM resources with this prefix."
  type        = string
  default     = "satark"
}

variable "repository" {
  description = "GitHub repository in owner/name form."
  type        = string
}

variable "default_branch" {
  description = "Branch whose pushes may run plan."
  type        = string
  default     = "main"
}

variable "apply_environments" {
  description = "Protected GitHub environments allowed to assume the apply role."
  type        = list(string)
  default     = ["dev", "prod"]
}

variable "state_bucket" {
  description = "Name of the Terraform state bucket."
  type        = string
}

variable "create_oidc_provider" {
  description = "Create the account's GitHub OIDC provider."
  type        = bool
  default     = true
}

variable "oidc_provider_arn" {
  description = "Existing provider ARN when create_oidc_provider is false."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags applied to every resource."
  type        = map(string)
}
