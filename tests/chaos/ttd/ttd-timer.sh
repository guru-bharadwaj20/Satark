#!/usr/bin/env bash
# Facilitator stopwatch for time-to-diagnose trials.
#
#   tests/chaos/ttd/ttd-timer.sh <game-day> <condition:opsbot|manual> <responder-alias>
#
# Press Enter when the fault is injected, then when the alert fires, then when
# the responder states the correct root cause. Appends one row to results.csv.
set -euo pipefail
GD="${1:?game day, e.g. gd-04}"
COND="${2:?opsbot or manual}"
WHO="${3:?responder alias (no real names)}"
[[ "$COND" == opsbot || "$COND" == manual ]] || { echo "condition must be opsbot or manual" >&2; exit 2; }
CSV="$(dirname "$0")/results.csv"
[[ -f "$CSV" ]] || echo "date,game_day,condition,responder,alert_s,diagnose_s,correct,notes" > "$CSV"

read -r -p "Enter when the fault is injected... " _
t0=$(date +%s)
read -r -p "Enter when the first alert fires... " _
t1=$(date +%s)
read -r -p "Enter when the responder states the root cause... " _
t2=$(date +%s)
read -r -p "Was the root cause correct? [y/n] " ok
read -r -p "Notes (no commas): " notes

correct=false; [[ "$ok" == y* ]] && correct=true
echo "$(date -u +%F),$GD,$COND,$WHO,$((t1 - t0)),$((t2 - t1)),$correct,${notes//,/;}" >> "$CSV"
echo "alert after $((t1 - t0))s, diagnosed $((t2 - t1))s after alert"
