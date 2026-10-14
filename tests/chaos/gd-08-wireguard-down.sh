#!/usr/bin/env bash
# Game day 8: WireGuard tunnel between the AWS edge proxy and on-prem down.
# Runs `wg-quick down wg0` on the edge EC2 instance through SSM, so the
# public endpoint stays up but cannot reach the cluster.
#   EDGE_INSTANCE_ID=i-0123 tests/chaos/gd-08-wireguard-down.sh inject
source "$(dirname "$0")/lib.sh"
EDGE="${EDGE_INSTANCE_ID:?edge EC2 instance id}"
REGION="${REGION:-ap-south-1}"
WG_IF="${WG_IF:-wg0}"

edge() {
  local id
  id=$(aws ssm send-command --region "$REGION" --instance-ids "$EDGE" \
    --document-name AWS-RunShellScript --parameters "commands=[\"$1\"]" \
    --query Command.CommandId --output text)
  aws ssm wait command-executed --region "$REGION" --command-id "$id" --instance-id "$EDGE" || true
  aws ssm get-command-invocation --region "$REGION" --command-id "$id" --instance-id "$EDGE" \
    --query StandardOutputContent --output text
}

case "${1:-}" in
  inject)
    confirm "This takes the $WG_IF tunnel down on the edge proxy (public traffic will fail)."
    edge "wg show $WG_IF latest-handshakes" | tee -a "$GD_LOG"
    event "inject: wg-quick down $WG_IF on $EDGE"
    # systemd would bring it back on reboot only; keep the unit stopped.
    edge "systemctl stop wg-quick@$WG_IF"
    probe 300
    ;;
  observe)
    edge "wg show $WG_IF 2>&1; systemctl is-active wg-quick@$WG_IF" | tee -a "$GD_LOG"
    ;;
  revert)
    event "revert: start wg-quick@$WG_IF"
    edge "systemctl start wg-quick@$WG_IF && sleep 5 && wg show $WG_IF latest-handshakes" | tee -a "$GD_LOG"
    probe 30
    ;;
  *) usage ;;
esac
