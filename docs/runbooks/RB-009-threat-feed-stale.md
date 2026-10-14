# RB-009: Threat-feed sync is stale

| Field | Value |
|---|---|
| Service | `feed-sync` (CronJob), `url-intel` |
| Namespace | `satark` (jobs), `satark-data` (Valkey, Postgres) |
| Alert | `ThreatFeedStale` |
| Severity | warning after 2 h, critical after 6 h |

The alert fires on:

```promql
time() - max by (feed) (satark_threat_feed_last_success_timestamp_seconds) > 7200
```

`url-intel` reads this gauge from the Valkey hash `satark:feed:meta`, which `feed-sync` updates only after a feed snapshot has been loaded completely. The gauge can also go missing, for example after Valkey loses its data. Watch for that with `absent(satark_threat_feed_last_success_timestamp_seconds{feed="openphish"})`.

## Symptoms

- `ThreatFeedStale` is firing for one or more feeds (`urlhaus`, `openphish`).
- The Grafana "Link intelligence" dashboard shows the feed's last sync time not moving.
- Recent `feed-sync` Jobs are `Failed`, or no new Jobs have been created.
- `satark_threat_feed_entries` is flat or missing.

## Impact

- Newly reported phishing and malware URLs are not matched. Older entries keep working, so links that were already listed are still flagged.
- Verdicts rely more heavily on heuristics (domain age, lookalikes, CT, Safe Browsing). This means more false negatives for links on established but compromised hosts.
- Users are not told anything directly, and the "No red flags found" wording stays the same. Treat this as a detection-quality incident, not an outage.

## Diagnosis

1. Check the CronJob and its recent Jobs:

   ```sh
   kubectl -n satark get cronjob feed-sync
   kubectl -n satark get jobs -l app.kubernetes.io/name=feed-sync --sort-by=.metadata.creationTimestamp | tail
   ```

   - `SUSPEND True`: someone paused it. Go to Fix A.
   - `LAST SCHEDULE` is old and there are no new Jobs: the controller skipped runs (`startingDeadlineSeconds` was missed, or `concurrencyPolicy: Forbid` is blocked by a stuck Job). Go to Fix B.

2. Read the logs of the latest Job (JSON lines, one per feed):

   ```sh
   kubectl -n satark logs job/$(kubectl -n satark get jobs -l app.kubernetes.io/name=feed-sync \
     --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')
   ```

   | Log message | Meaning | Go to |
   |---|---|---|
   | `fetch failed ... status 4xx` | Feed URL moved, was blocked or rate-limited | Fix C |
   | `fetch failed ... feed returned too few entries` | Upstream served an empty or truncated file, and we refused to overwrite good data | Fix C |
   | `fetch failed ... dial tcp / i/o timeout` | Egress blocked (NetworkPolicy, NAT, DNS) | Fix D |
   | `sink setup failed ... valkey` / `store failed ... sink=valkey` | Valkey is down, out of memory or the password changed | Fix E |
   | `sink setup failed ... postgres` / `sink=postgres` | Postgres is down, credentials are wrong or a migration failed | Fix E |
   | Pod `OOMKilled` | The feed grew past the memory limit | Fix F |

3. Check the data directly in Valkey:

   ```sh
   kubectl -n satark-data exec -it deploy/valkey -- valkey-cli HGETALL satark:feed:meta
   kubectl -n satark-data exec -it deploy/valkey -- valkey-cli SCARD satark:feed:openphish:urls
   ```

   If you see leftover `satark:feed:*:tmp:*` keys, a load was interrupted. They expire on their own after one hour.

4. Confirm that `url-intel` can reach Valkey: `kubectl -n satark get pods -l app.kubernetes.io/name=url-intel`. If its `/readyz` is failing, Valkey is the problem. See RB-002 or the Valkey section of the platform runbooks.

## Fix

**A. CronJob suspended.** Find out why from the change log or ArgoCD history. Then resume it: `kubectl -n satark patch cronjob feed-sync -p '{"spec":{"suspend":false}}'`. Prefer reverting the Git change so that ArgoCD does not suspend it again.

**B. Runs skipped or stuck.** Delete the stuck Job: `kubectl -n satark delete job <name>`. Then run a sync by hand:

```sh
kubectl -n satark create job --from=cronjob/feed-sync feed-sync-manual-$(date +%s)
```

**C. Upstream problem.** Open the feed URL from a workstation (`curl -sI https://openphish.com/feed.txt`, `curl -sI https://urlhaus.abuse.ch/downloads/text_online/`). If the URL has changed, update `feeds.Defaults` in `apps/feed-sync/internal/feeds/feeds.go` and ship a release. If you are rate-limited (429), wait. The schedule is every 30 minutes, which is within both providers' guidance. Do **not** lower `MinEntries` to force a load.

**D. Egress blocked.** Check recent NetworkPolicy and Kyverno changes. `feed-sync` needs egress to TCP 443 and to DNS. Test from inside the namespace:

```sh
kubectl -n satark run egress-test --rm -it --image=curlimages/curl --restart=Never -- \
  curl -sS -o /dev/null -w '%{http_code}\n' https://openphish.com/feed.txt
```

**E. Storage problem.** Fix Valkey or Postgres first (see their runbooks), then run a manual sync (Fix B). If the credentials were rotated, check the `valkey-auth` and `feed-sync-db` Secrets (ExternalSecrets) and their sync status.

**F. OOMKilled.** Raise `resources.limits.memory` for `feed-sync` in the chart values (in steps of 128Mi), commit the change and let ArgoCD sync it. Then run a manual sync.

**Valkey data lost but Postgres intact.** A manual sync rebuilds Valkey from upstream. If upstream is down too, rebuild from Postgres:

```sh
psql "$DATABASE_URL" -Atc "SELECT url FROM threat_feed_entries WHERE feed='openphish' AND active" \
  | sed 's/^/SADD satark:feed:openphish:urls /' | valkey-cli --pipe
```

## Verify

- The manual Job completes, and its logs show `feed synced` with a plausible `entries` count for each feed (thousands for URLhaus, hundreds or more for OpenPhish).
- `HGET satark:feed:meta openphish:last_success` shows the current time.
- `time() - satark_threat_feed_last_success_timestamp_seconds` drops below 1800 and the alert resolves.
- A URL from the current OpenPhish feed, scanned through `/v1/scan/url`, returns `dangerous` with the reason `listed as malicious by openphish`.

## Escalation

- Upstream outage longer than 24 h: tell the project maintainers in the on-call channel and consider enabling another feed (for example PhishTank) in `feeds.Defaults`.
- Valkey or Postgres data loss: escalate to the platform on-call (see RB-003 for Postgres).
- If the cause is unclear after 1 h of a critical alert, page the secondary on-call.

## Prevention

- Keep the `MinEntries` guard and the atomic `RENAME` swap so that a bad download never replaces good data.
- Alert on staleness and on `absent()`, not on Job failures alone. A suspended or never-scheduled CronJob produces no failures.
- Record feed URL changes from abuse.ch and OpenPhish announcements in the sprint notes.
- Add a test for every new feed parser to `apps/feed-sync/internal/feeds` with a recorded fixture.
- Review the `feed-sync` memory limit every quarter against `satark_threat_feed_entries`.
