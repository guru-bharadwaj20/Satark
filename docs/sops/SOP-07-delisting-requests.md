# SOP-07: Review a delisting request

**Owner:** maintainer on reputation duty · **Target turnaround:** 7 days · **Service:** `reputation` (namespace `satark`)

Community reports can be wrong. A shopkeeper's number can be mass-reported after a dispute, or a recycled SIM can inherit an old number's history. Anyone can ask for a number, UPI ID or domain to be reviewed. This SOP is how a reviewer decides.

## Principles

- Satark never calls anyone a "scammer". A delisting decision is about whether the reports are reliable, not about guilt.
- When in doubt, **approve**. Wrongly warning people about an innocent person causes real harm; the threshold means genuine fraud will quickly be re-reported.
- Never ask the requester for Aadhaar, PAN, bank statements or OTPs. A business registration, a public website, or a reasonable explanation is enough.
- Contact details in a request are personal data: don't copy them into tickets or chat.

## 1. Get a shell with the admin token

```bash
export REP_TOKEN=$(kubectl -n satark get secret reputation-admin -o jsonpath='{.data.token}' | base64 -d)
kubectl -n satark port-forward svc/reputation 8083:8083 &
```

## 2. List pending requests

```bash
curl -s -H "Authorization: Bearer $REP_TOKEN" \
  'http://localhost:8083/v1/admin/delisting-requests?status=pending' | jq '.requests[] | {id, kind, subject, reason, created_at}'
```

Subjects are masked (`98XXXXXX21`, `xy***@examplebank`). Reports are stored as hashes, so you cannot see who reported.

## 3. Check what the public currently sees

You need the full value, which the requester gave in their request form. Ask them only if it is unclear.

```bash
curl -s 'http://localhost:8083/v1/lookup?kind=phone&value=<value>' | jq
```

## 4. Decide

| Situation | Decision |
|---|---|
| Plausible explanation (business number, recycled SIM, personal dispute), no strong contrary evidence | approve |
| Domain now serves a legitimate, verifiable business and the old content is gone | approve |
| Request looks automated, abusive, or is for a subject with `100+` reporters and no explanation | reject, with a note |
| The subject currently matches a threat feed or an active police advisory | reject, with a note citing the source |

Approve:

```bash
curl -s -X POST -H "Authorization: Bearer $REP_TOKEN" -H 'Content-Type: application/json' \
  -d '{"decision":"approve","note":"business number, verified public listing"}' \
  http://localhost:8083/v1/admin/delisting-requests/<id> | jq
```

Reject: same command with `"decision":"reject"`. Approval deletes every existing report about the subject. This cannot be undone.

## 5. Reply to the requester

If they left a contact, reply from the project address with the decision only. Do not share report counts or categories beyond what the public lookup shows.

## 6. Verify

```bash
# The request is no longer pending
curl -s -H "Authorization: Bearer $REP_TOKEN" \
  'http://localhost:8083/v1/admin/delisting-requests?status=pending' | jq '[.requests[] | select(.id == <id>)] | length'   # expect 0

# After approval, the public lookup reveals nothing
curl -s 'http://localhost:8083/v1/lookup?kind=phone&value=<value>' | jq -r .status   # expect insufficient_reports

# The decision was counted
curl -s http://localhost:8083/metrics | grep satark_reputation_delisting_decisions_total
```
