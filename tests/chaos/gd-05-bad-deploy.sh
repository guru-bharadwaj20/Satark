#!/usr/bin/env bash
# Game day 5: ship a bad deploy and roll back.
#
# Simulates a release of gateway-api whose config points at the wrong
# text-analyzer port. The pod still passes /readyz (it does not check
# dependencies), so the rollout "succeeds" and only the error rate shows it.
#
# Argo CD self-heal would undo a plain kubectl edit within seconds, so the
# script pauses auto-sync on the Application first, as a hotfix applied
# outside Git would in real life.
#   APP=gateway-api tests/chaos/gd-05-bad-deploy.sh inject
source "$(dirname "$0")/lib.sh"
APP="${APP:-gateway-api}"
NS=satark
BAD_ENV="${BAD_ENV:-TEXT_ANALYZER_URL=http://text-analyzer.satark.svc:9999}"

case "${1:-}" in
  inject)
    require_context
    confirm "This rolls out a broken $APP with Argo CD auto-sync paused."
    snapshot before
    kubectl -n "$NS" rollout history "deploy/$APP" | tee -a "$GD_LOG"
    event "inject: pause auto-sync on argocd/$APP"
    kubectl -n argocd patch application "$APP" --type merge -p '{"spec":{"syncPolicy":{"automated":null}}}'
    event "inject: set env $BAD_ENV"
    kubectl -n "$NS" set env "deploy/$APP" "$BAD_ENV"
    kubectl -n "$NS" annotate "deploy/$APP" kubernetes.io/change-cause="gd-05 bad release" --overwrite
    kubectl -n "$NS" rollout status "deploy/$APP" --timeout=5m
    event "bad rollout complete"
    probe 300
    ;;
  observe)
    kubectl -n "$NS" rollout history "deploy/$APP" | tee -a "$GD_LOG"
    kubectl -n argocd get application "$APP" -o jsonpath='{.status.sync.status} {.status.health.status}{"\n"}' | tee -a "$GD_LOG"
    ;;
  rollback)
    # What the responder is expected to do (one of):
    event "rollback: kubectl rollout undo deploy/$APP"
    kubectl -n "$NS" rollout undo "deploy/$APP"
    kubectl -n "$NS" rollout status "deploy/$APP" --timeout=5m
    ;;
  revert)
    event "revert: re-enable auto-sync (Git is the source of truth again)"
    kubectl -n argocd patch application "$APP" --type merge \
      -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'
    kubectl -n argocd annotate application "$APP" argocd.argoproj.io/refresh=hard --overwrite
    kubectl -n "$NS" rollout status "deploy/$APP" --timeout=5m
    snapshot after
    ;;
  *) echo "usage: $0 inject|observe|rollback|revert" >&2; exit 2 ;;
esac
