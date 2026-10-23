# CloudWatch agent support for the edge. The agent itself and the
# WireGuard handshake-age publisher are installed by the Ansible role
# infra/ansible/roles/cloudwatch-agent. It emits the gauge
# wireguard_handshake_age_seconds in the metric namespace below.

locals {
  metric_namespace = "Satark/Edge"
  agent_log_groups = ["haproxy", "syslog"]
}

resource "aws_iam_role_policy_attachment" "cloudwatch_agent" {
  role       = aws_iam_role.edge.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# Pre-create the agent's log groups so retention is managed here, not by the agent.
resource "aws_cloudwatch_log_group" "agent" {
  #checkov:skip=CKV_AWS_338:Retention is configurable; 30 days keeps cost low for a hobby-scale edge
  #checkov:skip=CKV_AWS_158:Proxy and syslog lines hold no secrets; KMS adds cost
  for_each = toset(local.agent_log_groups)

  name              = "/satark/${var.name}/${each.value}"
  retention_in_days = var.log_retention_days
  tags              = var.tags
}
