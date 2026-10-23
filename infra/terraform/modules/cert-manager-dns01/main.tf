# IAM identity for cert-manager's Route 53 DNS-01 solver.
#
# Permissions follow the cert-manager docs and are scoped to the one hosted
# zone. ChangeResourceRecordSets is further limited to TXT records named
# _acme-challenge.*, so a leaked key cannot repoint the site.
#
# The cluster is on-prem (no IRSA), so this is an IAM user. Terraform
# deliberately does NOT create an access key, because the secret would end up
# in state. Create it once by hand and store it as a sealed secret:
#
#   aws iam create-access-key --user-name <user_name>
#   kubectl -n cert-manager create secret generic route53-credentials \
#     --from-literal=secret-access-key=... --dry-run=client -o yaml | kubeseal ...

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

locals {
  zone_arn = "arn:${data.aws_partition.current.partition}:route53:::hostedzone/${var.zone_id}"
}

data "aws_iam_policy_document" "dns01" {
  statement {
    sid       = "GetChange"
    actions   = ["route53:GetChange"]
    resources = ["arn:${data.aws_partition.current.partition}:route53:::change/*"]
  }

  statement {
    sid       = "ListZoneRecords"
    actions   = ["route53:ListResourceRecordSets"]
    resources = [local.zone_arn]
  }

  statement {
    sid       = "ChallengeRecordsOnly"
    actions   = ["route53:ChangeResourceRecordSets"]
    resources = [local.zone_arn]

    condition {
      test     = "ForAllValues:StringEquals"
      variable = "route53:ChangeResourceRecordSetsRecordTypes"
      values   = ["TXT"]
    }

    condition {
      test     = "ForAllValues:StringLike"
      variable = "route53:ChangeResourceRecordSetsNormalizedRecordNames"
      values   = ["_acme-challenge.*"]
    }
  }

  statement {
    sid       = "FindZone"
    actions   = ["route53:ListHostedZonesByName"]
    resources = ["*"]
  }
}

resource "aws_iam_policy" "dns01" {
  name        = "${var.name}-cert-manager-dns01"
  description = "cert-manager DNS-01 for hosted zone ${var.zone_id}"
  policy      = data.aws_iam_policy_document.dns01.json
  tags        = var.tags
}

resource "aws_iam_user" "cert_manager" {
  #checkov:skip=CKV_AWS_273:On-prem cluster has no SSO or IRSA; a scoped machine user is required
  count = var.create_user ? 1 : 0

  name = "${var.name}-cert-manager"
  path = "/satark/"
  tags = var.tags
}

resource "aws_iam_user_policy_attachment" "cert_manager" {
  #checkov:skip=CKV_AWS_40:Single-purpose machine user; a group adds nothing
  count = var.create_user ? 1 : 0

  user       = aws_iam_user.cert_manager[0].name
  policy_arn = aws_iam_policy.dns01.arn
}
