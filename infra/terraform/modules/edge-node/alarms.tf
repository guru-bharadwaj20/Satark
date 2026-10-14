# Tunnel-down alarm with SNS email (task 5.24).
#
# WireGuard re-handshakes about every 2 minutes on a live tunnel (the client
# sends a keepalive every 25s), so a handshake age above
# tunnel_down_threshold_seconds for 3 consecutive minutes means the tunnel is
# down. Missing data counts as breaching: if the agent or the instance dies,
# you should hear about it too.

resource "aws_sns_topic" "alerts" {
  name              = "${var.name}-alerts"
  kms_master_key_id = "alias/aws/sns"
  tags              = var.tags
}

resource "aws_sns_topic_subscription" "email" {
  for_each = toset(var.alarm_emails)

  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = each.value
}

resource "aws_cloudwatch_metric_alarm" "tunnel_down" {
  alarm_name        = "${var.name}-wireguard-tunnel-down"
  alarm_description = "No WireGuard handshake from worker-1 for over ${var.tunnel_down_threshold_seconds}s. Public traffic cannot reach the cluster. Runbook: check `wg show` on the edge (SSM) and worker-1, and home internet."

  namespace   = local.metric_namespace
  metric_name = "wireguard_handshake_age_seconds"
  dimensions = {
    InstanceId  = aws_instance.edge.id
    metric_type = "gauge"
  }

  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 3
  datapoints_to_alarm = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.tunnel_down_threshold_seconds
  treat_missing_data  = "breaching"

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]

  tags = var.tags
}

# Also alert when the instance itself fails its status checks.
resource "aws_cloudwatch_metric_alarm" "instance_status" {
  alarm_name        = "${var.name}-status-check-failed"
  alarm_description = "Edge EC2 status check failing."

  namespace   = "AWS/EC2"
  metric_name = "StatusCheckFailed"
  dimensions = {
    InstanceId = aws_instance.edge.id
  }

  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 3
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 1
  treat_missing_data  = "missing"

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]

  tags = var.tags
}
