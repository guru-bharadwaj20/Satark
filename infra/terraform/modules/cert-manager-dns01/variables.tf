variable "name" {
  description = "Name prefix, e.g. satark-prod."
  type        = string
}

variable "zone_id" {
  description = "Route 53 hosted zone ID cert-manager may write challenge records to."
  type        = string
}

variable "create_user" {
  description = "Create an IAM user for the on-prem cluster. Set false to only create the policy."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags applied to every resource."
  type        = map(string)
}
