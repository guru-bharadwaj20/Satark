#!/usr/bin/env bash
# Game day 9: image pull failure.
# MODE=tag      - roll out a tag that does not exist (typo in a release)
# MODE=registry - block the registry from one node so cached images still run
#                 but any reschedule there fails to pull
#   APP=text-analyzer MODE=tag tests/chaos/gd-09-image-pull.sh inject
source "$(dirname "$0")/lib.sh"
APP="${APP:-text-analyzer}"
NS=satark
MODE="${MODE:-tag}"
NODE="${NODE:-worker-1}"
SSH="${SSH:-ssh}"
REGISTRY_HOST="${REGISTRY_HOST:-ghcr.io}"
STATE="${GD_LOG%.log}-image.txt"

case "${1:-}" in
  inject)
    require_context
    confirm "This breaks image pulls for $APP ($MODE)."
    snapshot before
    if [[ "$MODE" == tag ]]; then
      img=$(kubectl -n "$NS" get deploy "$APP" -o jsonpath='{.spec.template.spec.containers[0].image}')
      echo "$img" > "$STATE"
      kubectl -n argocd patch application "$APP" --type merge -p '{"spec":{"syncPolicy":{"automated":null}}}'
      bad="${img%:*}:v0.0.0-doesnotexist"
      event "inject: set image $bad"
      kubectl -n "$NS" set image "deploy/$APP" "*=$bad"
    else
      event "inject: blackhole $REGISTRY_HOST on $NODE and clear its cached $APP image"
      $SSH "$NODE" -- sudo bash -s <<REMOTE
echo "0.0.0.0 $REGISTRY_HOST" >> /etc/hosts
crictl images | awk '/$APP/ {print \$3}' | xargs -r crictl rmi || true
REMOTE
      kubectl -n "$NS" delete pod -l "app.kubernetes.io/name=$APP" --field-selector "spec.nodeName=$NODE"
    fi
    probe 180
    ;;
  observe)
    kubectl -n "$NS" get pods -l "app.kubernetes.io/name=$APP" -o wide | tee -a "$GD_LOG"
    kubectl -n "$NS" get events --field-selector reason=Failed --sort-by=.lastTimestamp | tail -5 | tee -a "$GD_LOG"
    ;;
  revert)
    if [[ "$MODE" == tag ]]; then
      event "revert: restore image $(cat "$STATE")"
      kubectl -n "$NS" set image "deploy/$APP" "*=$(cat "$STATE")"
      kubectl -n argocd patch application "$APP" --type merge \
        -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'
    else
      event "revert: unblock $REGISTRY_HOST on $NODE"
      $SSH "$NODE" -- sudo sed -i "/ $REGISTRY_HOST\$/d" /etc/hosts
    fi
    kubectl -n "$NS" rollout status "deploy/$APP" --timeout=5m
    snapshot after
    ;;
  *) usage ;;
esac
