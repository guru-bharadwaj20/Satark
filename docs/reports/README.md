# Monthly scam-trends reports

On the first working day of each month, publish an anonymised summary of the previous month's community reports as `docs/reports/YYYY-MM-scam-trends.md`.

## Privacy rules (enforced by the generator)

- Only aggregates by scam type and by what was reported (phone, UPI ID, website).
- No phone number, UPI ID, domain or reporter is ever listed.
- Any group with fewer than 10 independent reporters is merged into "Other / small groups"; if the whole month has fewer than 10, nothing is published.
- Counts are rounded (nearest 5, or 10 above 100).
- Reports are described as reports, not confirmed fraud.

## Generate

```bash
MONTH=$(date -u -d "$(date -u +%Y-%m-01) -1 month" +%Y-%m)
kubectl -n satark run trends-$MONTH --rm -i --restart=Never \
  --image=ghcr.io/guru-bharadwaj20/satark/reputation:latest \
  --overrides='{"spec":{"containers":[{"name":"t","image":"ghcr.io/guru-bharadwaj20/satark/reputation:latest","args":["trends","-month","'"$MONTH"'"],"env":[{"name":"DATABASE_URL","valueFrom":{"secretKeyRef":{"name":"satark-pg-app","key":"uri"}}}]}]}}' \
  > docs/reports/$MONTH-scam-trends.md
```

## Review before publishing

- [ ] Read the whole file. Nothing that identifies a person, number, UPI ID or website.
- [ ] Add one or two sentences of context if there was a notable pattern (e.g. electricity-bill scams around bill dates), without naming any company as a perpetrator.
- [ ] Commit as `docs(reports): scam trends for <Month YYYY>` and share the link in the launch groups and the bot's channel.

## Verify

```bash
# No 10-digit numbers or VPAs slipped in (expect no output)
grep -En '[6-9][0-9]{9}|[a-z0-9._-]+@[a-z]+' docs/reports/$MONTH-scam-trends.md
```
