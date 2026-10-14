# SSM agent and Session Manager access (task 5.11). Ubuntu AMIs ship the SSM
# agent as a snap; it only needs an instance profile with
# AmazonSSMManagedInstanceCore. Connect with:
#
#   aws ssm start-session --target <instance_id>
#
# Session logs go to CloudWatch Logs, encrypted in transit.

data "aws_partition" "current" {}

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "edge" {
  name               = "${var.name}-instance"
  description        = "Edge node instance role (SSM)"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  role       = aws_iam_role.edge.name
  policy_arn = "arn:${data.aws_partition.current.partition}:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "edge" {
  name = "${var.name}-instance"
  role = aws_iam_role.edge.name
  tags = var.tags
}

resource "aws_cloudwatch_log_group" "ssm_sessions" {
  name              = "/satark/${var.name}/ssm-sessions"
  retention_in_days = var.log_retention_days
  tags              = var.tags
}

data "aws_iam_policy_document" "session_logs" {
  statement {
    actions   = ["logs:CreateLogStream", "logs:PutLogEvents", "logs:DescribeLogStreams"]
    resources = ["${aws_cloudwatch_log_group.ssm_sessions.arn}:*"]
  }
  statement {
    actions   = ["logs:DescribeLogGroups"]
    resources = ["*"]
  }
}

resource "aws_iam_role_policy" "session_logs" {
  name   = "${var.name}-ssm-session-logs"
  role   = aws_iam_role.edge.id
  policy = data.aws_iam_policy_document.session_logs.json
}

# Session preferences for this stack's sessions (use with
# --document-name <name>). Shell runs as the unprivileged ssm-user.
resource "aws_ssm_document" "session_prefs" {
  name            = "${var.name}-session-prefs"
  document_type   = "Session"
  document_format = "JSON"

  content = jsonencode({
    schemaVersion = "1.0"
    description   = "Satark edge Session Manager preferences"
    sessionType   = "Standard_Stream"
    inputs = {
      cloudWatchLogGroupName      = aws_cloudwatch_log_group.ssm_sessions.name
      cloudWatchEncryptionEnabled = false
      cloudWatchStreamingEnabled  = true
      idleSessionTimeout          = "20"
      runAsEnabled                = false
      shellProfile = {
        linux = "cd ~ && exec bash"
      }
    }
  })

  tags = var.tags
}
