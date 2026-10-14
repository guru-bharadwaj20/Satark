# RB-010: WireGuard tunnel (AWS edge to on-prem)

**Alerts:** external probe failing (Route 53 health check / synthetic),
`SatarkWireGuardHandshakeStale`, traffic rate drop on `satark_http_requests_total`
while the cluster is healthy.

Public traffic path: Route 53 -> edge EC2 (TCP proxy on 443) -> `wg0` ->
on-prem peer -> MetalLB VIP -> Envoy Gateway.

## 1. Confirm it is the tunnel (3 minutes)

1. Cluster healthy? `kubectl satark doctor`. If not, this is not RB-010.
2. From the edge (SSM, no SSH keys needed):
   ```bash
   aws ssm start-session --target <edge-instance-id>
   sudo wg show wg0                    # latest handshake should be < 2 min
   systemctl status wg-quick@wg0
   ping -c3 <on-prem wg address>       # e.g. 10.99.0.2
   curl -sk --max-time 3 https://<metallb-vip>/healthz
   ```
3. On the on-prem peer:
   ```bash
   sudo wg show wg0
   systemctl status wg-quick@wg0
   ```

| Observation | Likely cause | Go to |
| --- | --- | --- |
| `wg-quick@wg0` inactive on either side | service stopped / crashed | 2 |
| Both up, no handshake for > 3 min | UDP 51820 blocked, endpoint IP changed, key mismatch | 3 |
| Handshake recent, ping fails | routing / AllowedIPs / firewall on peer | 4 |
| Ping works, curl to VIP fails | not the tunnel: MetalLB / Envoy (other runbooks) | — |

## 2. Service down

```bash
sudo systemctl restart wg-quick@wg0 && sudo wg show wg0
sudo journalctl -u wg-quick@wg0 -n 50 --no-pager
```

If it fails to start, check `/etc/wireguard/wg0.conf` syntax and that the
private key file is present (managed by Ansible on-prem and by Terraform
user-data on the edge).

## 3. No handshake

- **On-prem public IP changed** (home/office ISP): the edge has a fixed
  Elastic IP, so the on-prem side initiates. Make sure on-prem has
  `Endpoint = <edge EIP>:51820` and `PersistentKeepalive = 25`.
- **UDP blocked:** edge security group must allow UDP 51820 from anywhere
  (or the on-prem IP); check the on-prem router/NAT.
  `sudo tcpdump -ni eth0 udp port 51820` on the edge shows whether packets arrive.
- **Key mismatch** after a rotation: compare `wg show wg0 public-key` on each
  side with the `[Peer] PublicKey` on the other.
- **Clock skew** large enough breaks handshakes: `timedatectl` on both sides.

## 4. Handshake OK, no traffic

- `AllowedIPs` on the edge must include the on-prem service CIDR / VIP
  range; on-prem must include the edge tunnel address.
- On-prem peer must forward: `sysctl net.ipv4.ip_forward` = 1, and NAT/route
  to the MetalLB VIP exists (`ip route get <vip>`).
- Cilium host firewall or a NetworkPolicy may drop traffic from the tunnel
  CIDR; check `hubble observe --from-ip <edge wg ip> --verdict DROPPED`.

## 5. If the tunnel cannot be restored quickly

- Put up the maintenance page on the edge proxy so users get a fast,
  clear error instead of timeouts.
- If the outage will exceed the RTO, start SOP-05 (DR on AWS k3s) and move
  DNS to the DR instance.

## Verify

- `wg show wg0` handshake < 2 min on both sides, transfer counters rising.
- External probe green; `curl https://api.satark.example/healthz` from outside.
- Request rate back to normal in Grafana.
