# Temporary disaster-recovery environment. Apply only during a DR drill or a
# real outage, and destroy afterwards:
#
#   terraform -chdir=infra/terraform/envs/dr apply -var 'admin_cidrs=["203.0.113.4/32"]'
#   terraform -chdir=infra/terraform/envs/dr destroy
terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
  backend "s3" {
    bucket         = "satark-tfstate"
    key            = "envs/dr/terraform.tfstate"
    region         = "ap-south-1"
    dynamodb_table = "satark-tf-locks"
    encrypt        = true
  }
}

provider "aws" {
  region = var.region
}

module "dr" {
  source        = "../../modules/dr_k3s"
  backup_bucket = var.backup_bucket
  admin_cidrs   = var.admin_cidrs
  k3s_version   = var.k3s_version
}

variable "region" {
  type    = string
  default = "ap-south-1"
}

variable "backup_bucket" {
  type    = string
  default = "satark-backups-prod"
}

variable "admin_cidrs" {
  type = list(string)
}

variable "k3s_version" {
  type    = string
  default = "v1.31.2+k3s1"
}

output "dr_public_ip" {
  value = module.dr.public_ip
}

output "dr_instance_id" {
  value = module.dr.instance_id
}
