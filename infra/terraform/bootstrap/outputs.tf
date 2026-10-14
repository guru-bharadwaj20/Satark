output "state_bucket" {
  description = "Name of the Terraform state bucket. Use it in each stack's backend block."
  value       = aws_s3_bucket.state.bucket
}

output "backend_snippet" {
  description = "Backend block to paste into envs/<env>/backend.tf."
  value       = <<-EOT
    terraform {
      backend "s3" {
        bucket       = "${aws_s3_bucket.state.bucket}"
        key          = "<env>/terraform.tfstate"
        region       = "${var.region}"
        encrypt      = true
        use_lockfile = true
      }
    }
  EOT
}
