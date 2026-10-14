variable "name" {
  description = "Name prefix for DR resources"
  type        = string
  default     = "satark-dr"
}

variable "instance_type" {
  description = "EC2 instance type for the single-node k3s cluster"
  type        = string
  default     = "t3.xlarge"
}

variable "disk_gb" {
  description = "Root volume size in GiB"
  type        = number
  default     = 80
}

variable "k3s_version" {
  description = "k3s release to install, e.g. v1.31.2+k3s1"
  type        = string
  default     = "v1.31.2+k3s1"
}

variable "backup_bucket" {
  description = "S3 bucket holding Velero and CNPG backups (read-only access is granted)"
  type        = string
}

variable "admin_cidrs" {
  description = "CIDRs allowed to reach the Kubernetes API on 6443"
  type        = list(string)
  validation {
    condition     = !contains(var.admin_cidrs, "0.0.0.0/0")
    error_message = "Do not expose the DR Kubernetes API to the whole internet."
  }
}

variable "key_name" {
  description = "Optional EC2 key pair name; SSM Session Manager works without one"
  type        = string
  default     = null
}

variable "tags" {
  description = "Extra tags"
  type        = map(string)
  default     = {}
}
