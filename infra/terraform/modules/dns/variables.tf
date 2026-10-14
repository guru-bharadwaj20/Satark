variable "domain" {
  description = "DNS zone name, e.g. satark.example.in or dev.satark.example.in."
  type        = string
}

variable "environment" {
  description = "Environment name, used in the zone comment."
  type        = string
}

variable "create_zone" {
  description = "Create the hosted zone. Set false to manage records in an existing zone."
  type        = bool
  default     = true
}

variable "edge_ip" {
  description = "Elastic IP of the edge node."
  type        = string
}

variable "edge_hostnames" {
  description = "Labels under the domain that resolve to the edge (\"\" means the apex)."
  type        = list(string)
  default     = ["", "api", "app", "bot", "grafana", "argocd"]
}

variable "ttl" {
  description = "TTL for edge A records."
  type        = number
  default     = 300
}

variable "caa_issuers" {
  description = "CAs allowed to issue certificates for the domain."
  type        = list(string)
  default     = ["letsencrypt.org"]
}

variable "caa_iodef_email" {
  description = "Optional email for CAA violation reports."
  type        = string
  default     = null
}

variable "extra_records" {
  description = "Additional records (TXT verification, MX and so on)."
  type = list(object({
    name    = string
    type    = string
    ttl     = optional(number, 300)
    records = list(string)
  }))
  default = []
}

variable "tags" {
  description = "Tags applied to the hosted zone."
  type        = map(string)
}
