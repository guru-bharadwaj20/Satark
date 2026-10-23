# RB-012: LLM provider outage or cost spike

Covers `text-analyzer` (scam classification) and `opsbot` (alert diagnosis).
Alerts: `SatarkLLMFallbackHigh`, `SatarkLLMBudgetWarning`, `SatarkLLMBudgetExceeded`,
`SatarkLLMTokenSpike` (`gitops/platform/monitoring/rules/satark-llm.yaml`).

## Symptoms

- Outage: `SatarkLLMFallbackHigh` fires; scan answers use template explanations only;
  `satark_llm_requests_total{outcome=~"timeout|provider_error|circuit_open"}` rising;
  opsbot posts "LLM unavailable - raw context only".
- Cost spike: `SatarkLLMBudgetWarning`/`Exceeded` or `SatarkLLMTokenSpike` fires;
  opsbot posts "Daily LLM token budget reached".

## Impact

- **Users are still answered.** By design (ADR-005) scans fall back to rules-only
  scores; only the LLM nudge (max +15/-10) and LLM-written explanations are lost.
  Expect lower recall on messages the rules don't cover.
- opsbot diagnoses degrade to raw context plus runbook links; `/ask` and `/postmortem`
  are unavailable.
- A cost spike burns the AWS budget (Bedrock) or the hosted API's credit.

## Diagnosis

1. Which provider and outcome?

   ```promql
   sum by (provider, model, outcome) (rate(satark_llm_requests_total[10m]))
   ```

2. Outage or our side? Check latency and the provider's status page.

   ```promql
   histogram_quantile(0.95, sum by (le, provider) (rate(satark_llm_request_duration_seconds_bucket[10m])))
   ```

   ```bash
   kubectl -n satark logs deploy/text-analyzer --since=15m | grep -E 'llm (timeout|error)'
   # Local provider:
   kubectl -n satark get pods -l app.kubernetes.io/name=ollama
   kubectl -n satark logs deploy/ollama --since=15m | tail -50
   ```

   Hosted provider: HTTP 401/403 means an expired or revoked key; 429 means rate limit
   or credit exhausted; 5xx means a provider incident.

3. Cost spike: where are the tokens going?

   ```promql
   sum by (provider, model, direction) (increase(satark_llm_tokens_total[1h]))
   sum by (outcome) (increase(satark_llm_requests_total[1h]))   # low "cached" share?
   sum(rate(satark_http_requests_total{service="gateway-api"}[5m]))  # traffic surge?
   ```

   Common causes: a traffic surge or abuse (check API-key rate limits), a cache outage
   (Valkey down, so every scan calls the LLM), a prompt change that raised token count,
   an opsbot alert storm (check `satark_opsbot_alerts_skipped_total{reason="cooldown"}`).

## Fix

Pick the least invasive option that stops the bleeding:

- **Switch to the local provider** (outage or cost):

  ```bash
  kubectl -n satark set env deploy/text-analyzer SATARK_LLM_PROVIDER=ollama
  kubectl -n satark set env deploy/opsbot OPSBOT_LLM_PROVIDER=ollama \
    OPSBOT_LLM_URL=http://ollama.satark.svc.cluster.local:11434
  ```

  Then make the same change in `gitops/envs/<env>` so Argo CD does not revert it.

- **Disable the LLM entirely** (rules-only, zero cost): set
  `SATARK_LLM_PROVIDER=none` in the GitOps values (unknown provider means disabled;
  `get_provider_or_none()` then returns no provider and the classifier answers with
  `FallbackReason.DISABLED`).
- **Rotate a bad API key:** update the Secret through the secrets workflow and restart:
  `kubectl -n satark rollout restart deploy/text-analyzer deploy/opsbot`.
- **Cache outage:** fix Valkey first (`kubectl -n satark-data get pods`); LLM cost drops
  back once the hit ratio recovers.
- **Abuse:** revoke the offending API key in `gateway-api` and lower its rate limit.
- **opsbot alert storm:** raise `OPSBOT_COOLDOWN_S` or lower `OPSBOT_DAILY_TOKEN_BUDGET`.
- **Prompt regression:** revert the prompt commit; the eval CI gate must pass.

## Verify

```promql
satark:llm_fallback_ratio:rate10m < 0.05      # outage resolved
satark:llm_cost_usd:increase24h               # flattening
sum(rate(satark_llm_requests_total{outcome="cached"}[10m])) / sum(rate(satark_llm_requests_total[10m]))
```

Send a test scan through the API and confirm the reply has a risk level and reasons,
then run `/ask is the LLM healthy?` in the ops channel.

## Escalation

- Provider incident longer than 1 hour: stay on the local provider and note it in the
  ops channel; no further action needed.
- Spend above 3x the daily budget, or suspected leaked API key: page the maintainer,
  revoke the key at the provider immediately, then review AWS Budgets / CloudTrail.

## Prevention

- Keep the eval gate green and watch token count per scan in PRs.
- Keep cache TTL at 7 days and alert on Valkey availability.
- Review the price table (`SATARK_LLM_PRICES`) when changing models.
- Run a quarterly drill: set an invalid key in dev and confirm the fallback and alerts.
