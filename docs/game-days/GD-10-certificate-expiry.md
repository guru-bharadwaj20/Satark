# GD-10: Certificate expiry drill

| | |
| --- | --- |
| Status | **Planned (requires manual execution on the live cluster)** |
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

| Check | Detected by doctor | Alert fired (time) | Notes |
| --- | --- | --- | --- |
| Expiring TLS secret | | | |
| Broken issuer | | | |
| Auto-renewal observed | n/a | n/a | |
| kubeadm certs days left | | | |

## Actions

| Finding | Action | Owner | Done |
| --- | --- | --- | --- |
