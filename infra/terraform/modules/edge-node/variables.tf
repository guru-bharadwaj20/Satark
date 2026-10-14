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

variable "public_ingress_cidrs" {
  description = "CIDRs allowed on 80/443."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "wireguard_port" {
  description = "UDP port WireGuard listens on."
  type        = number
  default     = 51820
}

variable "wireguard_peer_cidrs" {
  description = "Source CIDRs allowed to reach WireGuard. Home ISPs often rotate IPs, so this defaults to anywhere; WireGuard drops unauthenticated packets silently."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "instance_type" {
  description = "Graviton instance type."
  type        = string
  default     = "t4g.micro"
}

variable "ami_id" {
  description = "AMI override. Defaults to the latest Ubuntu 24.04 arm64 image."
  type        = string
  default     = null
}

variable "root_volume_gb" {
  description = "Root EBS volume size in GiB."
  type        = number
  default     = 10
}

variable "ebs_kms_key_arn" {
  description = "KMS key for EBS encryption. Null uses the AWS-managed key."
  type        = string
  default     = null
}

variable "detailed_monitoring" {
  description = "Enable 1-minute EC2 detailed monitoring (billed)."
  type        = bool
  default     = false
}

variable "user_data" {
  description = "Optional cloud-init user data."
  type        = string
  default     = null
}

variable "tags" {
  description = "Tags applied to every resource. Must include Project, Environment, Owner and ManagedBy."
  type        = map(string)
}
