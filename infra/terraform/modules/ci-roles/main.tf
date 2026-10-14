# Two CI roles (task 5.7):
#
#   plan  - read-only. Assumable from pull requests and from main. Can read
#           every resource and the state, and write only the state lock files.
#   apply - write. Assumable only from jobs that run in a protected GitHub
#           environment (environment:<name>), which require reviewer approval.

terraform {
  required_version = ">= 1.9.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

data "aws_partition" "current" {}
data "aws_caller_identity" "current" {}

locals {
  state_arn   = "arn:${data.aws_partition.current.partition}:s3:::${var.state_bucket}"
  account_id  = data.aws_caller_identity.current.account_id
  policy_base = "arn:${data.aws_partition.current.partition}:iam::aws:policy"
}

data "aws_iam_policy_document" "plan_state" {
  statement {
    sid       = "ListState"
    actions   = ["s3:ListBucket", "s3:GetBucketVersioning"]
    resources = [local.state_arn]
  }

  statement {
    sid       = "ReadState"
    actions   = ["s3:GetObject"]
    resources = ["${local.state_arn}/*"]
  }

  # Plan takes the lock too, so it needs to create and remove *.tflock only.
  statement {
    sid       = "LockState"
    actions   = ["s3:PutObject", "s3:DeleteObject"]
    resources = ["${local.state_arn}/*.tflock"]
  }
}

data "aws_iam_policy_document" "apply" {
  #checkov:skip=CKV_AWS_109:IAM writes are constrained to satark-* resource ARNs and CI roles are denied
  statement {
    sid       = "State"
    actions   = ["s3:ListBucket", "s3:GetBucketVersioning"]
    resources = [local.state_arn]
  }

  statement {
    sid       = "StateObjects"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["${local.state_arn}/*"]
  }

  # PowerUserAccess excludes IAM. Allow IAM only on satark-prefixed names,
  # so the apply role cannot touch the CI roles or admin identities.
  statement {
    sid = "ScopedIAM"
    actions = [
      "iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:UpdateRole", "iam:TagRole", "iam:UntagRole",
      "iam:UpdateAssumeRolePolicy", "iam:PassRole",
      "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:PutRolePolicy", "iam:DeleteRolePolicy",
      "iam:GetRolePolicy", "iam:ListRolePolicies", "iam:ListAttachedRolePolicies", "iam:ListInstanceProfilesForRole",
      "iam:CreatePolicy", "iam:DeletePolicy", "iam:GetPolicy", "iam:GetPolicyVersion", "iam:ListPolicyVersions",
      "iam:CreatePolicyVersion", "iam:DeletePolicyVersion", "iam:TagPolicy", "iam:UntagPolicy",
      "iam:CreateInstanceProfile", "iam:DeleteInstanceProfile", "iam:GetInstanceProfile",
      "iam:AddRoleToInstanceProfile", "iam:RemoveRoleFromInstanceProfile", "iam:TagInstanceProfile",
      "iam:CreateUser", "iam:DeleteUser", "iam:GetUser", "iam:TagUser", "iam:UntagUser",
      "iam:AttachUserPolicy", "iam:DetachUserPolicy", "iam:ListAttachedUserPolicies", "iam:ListGroupsForUser",
      "iam:ListUserPolicies", "iam:ListAccessKeys",
    ]
    resources = [
      "arn:${data.aws_partition.current.partition}:iam::${local.account_id}:role/${var.name_prefix}-*",
      "arn:${data.aws_partition.current.partition}:iam::${local.account_id}:policy/${var.name_prefix}-*",
      "arn:${data.aws_partition.current.partition}:iam::${local.account_id}:instance-profile/${var.name_prefix}-*",
      "arn:${data.aws_partition.current.partition}:iam::${local.account_id}:user/satark/${var.name_prefix}-*",
    ]
  }

  statement {
    sid     = "ProtectCIRoles"
    effect  = "Deny"
    actions = ["iam:*"]
    resources = [
      "arn:${data.aws_partition.current.partition}:iam::${local.account_id}:role/${var.name_prefix}-ci-*",
    ]
  }

  statement {
    sid       = "CreateServiceLinkedRoles"
    actions   = ["iam:CreateServiceLinkedRole"]
    resources = ["*"]
  }
}

module "plan" {
  source = "../github-oidc-role"

  role_name            = "${var.name_prefix}-ci-plan"
  description          = "Terraform plan from GitHub Actions (read-only)"
  repository           = var.repository
  subject_claims       = ["pull_request", "ref:refs/heads/${var.default_branch}"]
  create_oidc_provider = var.create_oidc_provider
  oidc_provider_arn    = var.oidc_provider_arn
  managed_policy_arns  = ["${local.policy_base}/ReadOnlyAccess"]
  inline_policy_json   = data.aws_iam_policy_document.plan_state.json
  tags                 = var.tags
}

module "apply" {
  source = "../github-oidc-role"

  role_name            = "${var.name_prefix}-ci-apply"
  description          = "Terraform apply from protected GitHub environments"
  repository           = var.repository
  subject_claims       = [for e in var.apply_environments : "environment:${e}"]
  create_oidc_provider = false
  oidc_provider_arn    = module.plan.oidc_provider_arn
  managed_policy_arns  = ["${local.policy_base}/PowerUserAccess"]
  inline_policy_json   = data.aws_iam_policy_document.apply.json
  tags                 = var.tags
}
