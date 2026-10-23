# GD-10: Certificate expiry drill

| | |
| --- | --- |
| Status | Executed on 2026-10-18 (on-prem lab) |
| Facilitator | guru-bharadwaj20 |
| Responder(s) | agent (no human responder in the lab run) |
| Script | `tests/chaos/gd-10-cert-expiry.sh` |
| Runbook under test | RB-004 (certificates) |
| Expected duration | 75 minutes (includes watching one renewal) |

Expired certificates cause total outages with no warning in most
dashboards. This drill checks that every layer of detection works, in a
throwaway namespace so production certificates are never touched.

## Hypothesis

1. **TLS Secret expiring in 1 day:** `kubectl satark doctor` reports FAIL
   and the `SatarkCertificateExpiringSoon` alert (x509 exporter or
   `certmanager_certificate_expiration_timestamp_seconds`) fires.
2. **Certificate with a missing issuer:** stays `Ready=False`; the
   `CertManagerCertNotReady` alert fires within 10 minutes; doctor reports
   FAIL with the issuer error.
3. **1 hour Certificate with `renewBefore: 50m`:** cert-manager renews it
   about 10 minutes after issue; the Secret's serial number changes with no
   manual action.
4. **kubeadm certificates:** `kubeadm certs check-expiration` shows more
   than 30 days left on all control-plane certificates (they are renewed on
   every `kubeadm upgrade apply`, see SOP-01).

## Blast radius and abort criteria

- Scope: the `gd10-certs` namespace only. No abort criteria needed beyond
  "something outside the namespace changed".

## Procedure

1. `... inject`
2. `... observe` immediately, again after 15 minutes, and after 60 minutes.
3. Record serial numbers of `gd10-short-lived` across the run:
   `kubectl -n gd10-certs get secret gd10-short-lived -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -serial -enddate`
4. Responder walks RB-004 for the broken-issuer certificate as if it were
   production: `kubectl describe certificate`, `certificaterequest`,
   `order`/`challenge` (for ACME), cert-manager logs.
5. `... revert`.

## Results

Lab: cert-manager v1.16.2 (same chart and version as
`gitops/platform/cert-manager`). The lab has no Prometheus, so the alert
column could not be measured on the cluster. Instead, the two alerts were
written (see below) and tested with `promtool test rules` on the same
scenario. Observations were taken at +0, +12 and +25 minutes. With
`renewBefore: 50m` a renewal is due every 10 minutes, so 25 minutes covered
two of them, and the 60 minute observation was not needed.

| Check | Detected by doctor | Alert fired (time) | Notes |
| --- | --- | --- | --- |
| Expiring TLS secret | FAIL at +0 (`expires in 0 days (2026-10-19)`) | not measured: no Prometheus in the lab; the alert needs cert-manager metrics, so it never covers a hand-made Secret | doctor is the only check for Secrets that cert-manager does not manage |
| Broken issuer | FAIL at +0, but the message was only "Issuing certificate as Secret does not exist" | `CertManagerCertNotReady` did not exist; added, fires after 10 min in the promtool test | the cause was only on the CertificateRequest |
| Auto-renewal observed | n/a | n/a | renewed exactly on schedule, twice; three serials |
| kubeadm certs days left | n/a | n/a | 363 days on every leaf, CAs 9 years |

Renewal of `gd10-short-lived`. Every renewal landed on its `renewalTime`
to the second:

| Revision | Serial | notBefore | notAfter | Next renewal |
| --- | --- | --- | --- | --- |
| 1 | `D38C6289…7948` | 15:55:19 | 16:55:19 | 16:05:19 |
| 2 | `70B1D19E…47B9` | 16:05:19 | 17:05:19 | 16:15:19 |
| 3 | `1DA85A0A…E461` | 16:15:19 | 17:15:19 | 16:25:19 |

Broken issuer: the Certificate's condition said `Ready=False`, "Issuing
certificate as Secret does not exist". CertificateRequest
`gd10-broken-issuer-1` was `Approved=True`, `Ready=False`. Over 25 minutes
the cert-manager logs had only optimistic-locking requeues for it, and
nothing named the missing issuer. The reason is on the request
(`reason=Pending`, `Referenced "ClusterIssuer" not found:
clusterissuer.cert-manager.io "does-not-exist" not found`) and in its
`IssuerNotFound` events. The run did not save that text, so it was captured
again afterwards, in 15 seconds, in a scratch namespace that was then
deleted.

Timeline (UTC):

| Time | Event |
| --- | --- |
| 14:54:10 | First attempt. The Secret was never created (see below). Reverted at 15:54:18 and run again |
| 15:55:17 | Inject: Secret `gd10-expiring` (1 day), Certificates `gd10-short-lived` and `gd10-broken-issuer` |
| 15:55:19 | `gd10-short-lived` issued (revision 1); `gd10-broken-issuer-1` request created and left pending |
| 15:55:38 | Observe +0: doctor FAIL on all four objects |
| 16:05:19 | Renewal (revision 2), no action taken |
| 16:15:19 | Renewal (revision 3) |
| 16:20:02 | Observe +25: unchanged; broken issuer still pending |
| 16:20:10 | Revert: namespace deleted |

Caveats: this is a single-host lab, and the host was also running WSL GPU
jobs. GD-06 (container runtime failure) ran on worker-1 during this drill.
cert-manager runs on worker-2 and the drill touches no workloads, so the
timings are not affected.

## What we learned

- **Renewal works.** cert-manager renewed on the exact second of
  `renewalTime`, twice in a row, with no manual action. Hypothesis 3 holds.
- **The alerts in RB-004 did not exist.** RB-004 listed
  `SatarkCertificateExpiringSoon` and `CertManagerCertNotReady`, but no
  PrometheusRule in Git defined them. `scripts/check-alert-runbooks.sh`
  only checks the other direction (every alert has a runbook), so nothing
  caught it. Both alerts are now in `gitops/platform/monitoring/rules/satark-alerts.yaml`,
  under a `satark-certificates` group, built on cert-manager's own metrics
  (its ServiceMonitor was already enabled):
  - `SatarkCertificateExpiringSoon` (critical, 15 min): less than 7 days
    left and the renewal time has already passed. A plain "expires in
    under 7 days" rule would fire forever on short-lived certificates like
    `gd10-short-lived`.
  - `CertManagerCertNotReady` (warning, 10 min): Ready is not True.
  - Checked with `promtool check rules` and a `promtool test rules` case
    built from this drill: short-lived and renewing stays silent, stuck
    renewal fires, missing issuer fires.
- **Doctor had a false FAIL.** It judged every certificate on fixed
  thresholds (FAIL under 7 days), so a healthy 1-hour certificate and its
  Secret always failed. Doctor now skips the expiry check for a Ready
  Certificate whose `renewalTime` is still ahead, and for that
  Certificate's Secret. It warns when renewal is more than 1 hour overdue.
- **Doctor pointed at the symptom.** For the broken issuer it printed the
  Certificate's message, which does not name the cause. It now appends the
  failing CertificateRequest's message ("Referenced ClusterIssuer not
  found"). RB-004 step 2 now says the cause is on the request, and not in
  the logs at the default level.
- **kubeadm certificates are fine** (363 days left, CAs 9 years). They are
  renewed by the Kubernetes upgrade anyway.
- **Script bug on Windows:** the first attempt created the Certificates but
  not the Secret. Under Git Bash, MSYS rewrote `-subj "/CN=..."` into a
  Windows path, and `openssl` stderr went to `/dev/null`, so the inject
  carried on silently. The script now converts the temp dir with
  `cygpath -m`, sets `MSYS_NO_PATHCONV=1`, and no longer hides openssl
  errors. Both are no-ops on Linux.

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
| Certificate alerts named in RB-004 did not exist | Added `SatarkCertificateExpiringSoon` and `CertManagerCertNotReady`; promtool-tested | guru-bharadwaj20 | ✅ |
| Doctor FAILs short-lived certificates that renew fine | Use `renewalTime` for Ready Certificates; skip their Secrets; warn when renewal is overdue; unit tests | guru-bharadwaj20 | ✅ |
| Doctor and RB-004 did not surface the issuer error | Doctor quotes the failing CertificateRequest; RB-004 step 2 updated | guru-bharadwaj20 | ✅ |
| openssl failed silently under Git Bash | `cygpath -m` + `MSYS_NO_PATHCONV=1`; stderr kept | guru-bharadwaj20 | ✅ |
| Nothing checks that runbook alert names exist | Extend `check-alert-runbooks.sh` with an allow-list for upstream alerts (CNPG, kube-prometheus, Velero) | guru-bharadwaj20 | |
| Hand-made TLS Secrets have no alert | Deploy an x509 exporter, or rely on doctor in the daily check | guru-bharadwaj20 | |
| Alert timing untested on a real Prometheus | Re-run on the cloud cluster | guru-bharadwaj20 | |
