#!/usr/bin/env bash
# Roll back a GitOps change with `git revert` and time how long Argo CD takes
# to bring the cluster back to the previous image.
#
# Usage: scripts/rollback-drill.sh <commit-to-revert> <service> <env>
#   e.g. scripts/rollback-drill.sh 1a2b3c4 gateway-api dev
#
# Needs: git (push access to main or a branch with auto-merge), kubectl
# pointed at the cluster, and optionally the argocd CLI. Writes a result line
# to docs/game-days/rollback-timings.csv.
set -euo pipefail

commit=${1:?commit to revert}
svc=${2:?service}
env=${3:-dev}
app="${svc}-${env}"
ns=$([ "$env" = prod ] && echo satark || echo satark-dev)
timeout=${TIMEOUT:-600}
root=$(git rev-parse --show-toplevel)
values="gitops/envs/$env/apps/$svc/values.yaml"
csv="$root/docs/game-days/rollback-timings.csv"

log() { printf '%s %s\n' "$(date -u +%H:%M:%S)" "$*"; }
now() { date +%s; }

cd "$root"
git fetch -q origin main
bad_tag=$(git show "$commit:$values" | awk '/^  tag:/{gsub(/"/,"",$2); print $2}')
good_tag=$(git show "$commit^:$values" | awk '/^  tag:/{gsub(/"/,"",$2); print $2}')
[ -n "$good_tag" ] || { echo "could not read previous tag from $values"; exit 1; }
log "reverting $commit: $svc ($env) $bad_tag -> $good_tag"

t0=$(now)
git revert --no-edit "$commit"
git push origin HEAD:main
t_push=$(now)
log "revert pushed after $((t_push - t0))s"

# Ask Argo CD to look now instead of waiting for the 3-minute poll.
if command -v argocd >/dev/null 2>&1; then
  argocd app get "$app" --refresh >/dev/null || true
else
  kubectl -n argocd annotate application "$app" argocd.argoproj.io/refresh=normal --overwrite >/dev/null || true
fi

deadline=$((t_push + timeout))
synced=""
while [ "$(now)" -lt "$deadline" ]; do
  img=$(kubectl -n "$ns" get deploy "$svc" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)
  if [ -z "$synced" ] && [[ "$img" == *":$good_tag" ]]; then
    synced=$(now); log "Argo CD applied $good_tag after $((synced - t_push))s"
  fi
  if [ -n "$synced" ] && kubectl -n "$ns" rollout status deploy/"$svc" --timeout=5s >/dev/null 2>&1; then
    t_done=$(now)
    total=$((t_done - t0))
    log "rollout complete: total ${total}s (push $((t_push - t0))s, sync $((synced - t_push))s, rollout $((t_done - synced))s)"
    [ -f "$csv" ] || echo "date_utc,service,env,from_tag,to_tag,push_s,sync_s,rollout_s,total_s" > "$csv"
    echo "$(date -u +%FT%TZ),$svc,$env,$bad_tag,$good_tag,$((t_push - t0)),$((synced - t_push)),$((t_done - synced)),$total" >> "$csv"
    exit 0
  fi
  sleep 2
done
log "timed out after ${timeout}s"
exit 1
