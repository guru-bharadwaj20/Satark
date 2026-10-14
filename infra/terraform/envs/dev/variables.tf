variable "region" {
  description = "AWS region."
  type        = string
  default     = "ap-south-1"
}

variable "name_prefix" {
  description = "Prefix for resource names."
  type        = string
  default     = "satark"
}

variable "environment" {
  description = "Environment name."
  type        = string

  validation {
    condition     = contains(["dev", "prod"], var.environment)
    error_message = "environment must be dev or prod."
  }
}

variable "owner" {
  description = "Owner tag value."
  type        = string
  default     = "guru-bharadwaj20"
}

variable "domain" {
  description = "DNS zone for this environment."
  type        = string
}

variable "create_zone" {
  description = "Create the hosted zone in this stack."
  type        = bool
  default     = true
}

variable "edge_vpc_cidr" {
  description = "Edge VPC CIDR."
  type        = string
}

variable "edge_instance_type" {
  description = "Edge Graviton instance type."
  type        = string
  default     = "t4g.micro"
}

variable "wireguard_peer_cidrs" {
  description = "Source CIDRs allowed to reach the WireGuard port."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "velero_retention_days" {
  description = "Days to keep Velero backups."
  type        = number
  default     = 30
}

variable "screenshot_retention_days" {
  description = "Days to keep sandbox screenshots."
  type        = number
  default     = 7
}

variable "force_destroy_buckets" {
  description = "Allow destroying non-empty buckets."
  type        = bool
  default     = false
}
