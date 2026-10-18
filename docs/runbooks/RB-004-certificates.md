# RB-004: Certificates

**Alerts:** `SatarkCertificateExpiringSoon`, `CertManagerCertNotReady`,
`KubeClientCertificateExpiration`, browser/Telegram TLS errors reported by users
**Quick check:** `kubectl satark doctor` (certificates section)

## 1. Which certificate?

```bash
kubectl satark doctor --json | jq '.results[] | select(.check=="certificates")'
kubectl get certificates -A
```

| Kind | Managed by | Go to |
| --- | --- | --- |
| Public API / PWA (`*.satark.example`) | cert-manager, Let's Encrypt via DNS-01 on Route 53 | 2 |
| Internal (webhooks, NATS, etc.) | cert-manager, self-signed CA issuer | 2 |
| Kubernetes control plane (apiserver, etcd, kubelet client) | kubeadm | 3 |
| WireGuard | keys, not certificates; see RB-010 | — |

## 2. cert-manager certificate not renewing

Walk the chain from Certificate down to the ACME challenge:

```bash
kubectl -n <ns> describe certificate <name>
kubectl -n <ns> describe certificaterequest -l cert-manager.io/certificate-name=<name>
kubectl -n <ns> describe order,challenge     # ACME only
kubectl -n cert-manager logs deploy/cert-manager --since=1h | grep <name>
```

The cause is usually on the CertificateRequest, not the Certificate: its
`Ready` condition and events (e.g. `IssuerNotFound`). The Certificate only
says "Issuing certificate as Secret does not exist", and at the default log
level the cert-manager logs did not name the missing issuer at all (GD-10).
`kubectl satark doctor` quotes the request's message for you.

Common causes:

- **Issuer missing or not Ready:** `kubectl get clusterissuer`. Fix the issuer
  first; Certificates retry with backoff.
- **DNS-01 fails:** Route 53 credentials (`route53-credentials` Secret) or IAM
  policy wrong; or the TXT record is not visible from outside. Check with
  `dig TXT _acme-challenge.api.satark.example @8.8.8.8`.
- **Let's Encrypt rate limit** (`too many certificates already issued`):
  wait, or temporarily switch the issuer to the staging endpoint while
  debugging. Do not loop deleting Certificates.
- **Stuck after fixing:** force renewal with
  `kubectl cert-manager renew -n <ns> <name>` (or delete the latest failed
  CertificateRequest).

### Emergency: public certificate already expired

1. Make renewal work (above) if possible; it takes 1 to 3 minutes once DNS
   validation passes.
2. If it cannot be fixed quickly, issue a certificate manually with certbot
   using the DNS-01 Route 53 plugin from an operator laptop and load it:
   `kubectl -n <ns> create secret tls <secretName> --cert fullchain.pem --key privkey.pem --dry-run=client -o yaml | kubectl apply -f -`.
   cert-manager will take over on the next successful renewal.
3. Envoy Gateway picks up the new Secret automatically; verify with
   `openssl s_client -connect api.satark.example:443 -servername api.satark.example </dev/null | openssl x509 -noout -enddate`.

## 3. kubeadm control-plane certificates

```bash
ssh cp1 sudo kubeadm certs check-expiration
```

They last one year and are renewed by every `kubeadm upgrade apply`
(SOP-01). If an upgrade has not happened in time:

```bash
ssh cp1 sudo kubeadm certs renew all
ssh cp1 sudo systemctl restart kubelet
# static pods must restart to load new certs:
ssh cp1 'sudo crictl ps --name "kube-apiserver|kube-controller-manager|kube-scheduler|etcd" -q | xargs sudo crictl stop'
ssh cp1 sudo cp /etc/kubernetes/admin.conf ~/.kube/config   # then copy to operators
```

Kubelet client certificates rotate automatically (`rotateCertificates: true`);
if a node shows `x509: certificate has expired` in kubelet logs, re-join it
(SOP-02).

## Verify

- `kubectl satark doctor` certificates check is OK.
- `kubectl get certificates -A` all `READY=True`.
- External check of the public endpoint end date (command above).
