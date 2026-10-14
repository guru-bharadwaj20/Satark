variable "name" {
  description = "Name prefix for edge resources, e.g. satark-prod-edge."
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for the edge VPC."
  type        = string
  default     = "10.60.0.0/16"

  validation {
    condition     = can(cidrhost(var.vpc_cidr, 0))
    error_message = "vpc_cidr must be a valid IPv4 CIDR."
  }
}

variable "availability_zone" {
  description = "AZ for the public subnet. Defaults to the first available AZ."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags applied to every resource. Must include Project, Environment, Owner and ManagedBy."
  type        = map(string)
}
