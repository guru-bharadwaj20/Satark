# Satark architecture

This document explains how Satark is put together: the request paths, where data lives, how untrusted links are isolated, and how the platform is operated. Diagrams are Mermaid and render on GitHub.

## 1. Context

```mermaid
flowchart LR
  user([Person with a suspicious message])
  tg[Telegram]
  pwa[PWA / Android share menu]
  partner[Third-party app<br/>with API key]
  satark[[Satark]]
  llm[(LLM provider<br/>redacted text only)]
  feeds[(Threat feeds)]
  gov([1930 · cybercrime.gov.in · Chakshu])

  user --> tg --> satark
  user --> pwa --> satark
  partner --> satark
  satark --> llm
  feeds --> satark
  satark -. ready-to-copy report details .-> user
  user -. reports officially .-> gov
```

Satark never reports to authorities on a user's behalf; it gives the user the details and the right channel.

## 2. Containers

```mermaid
flowchart TB
  subgraph edge[Edge]
    ing[Ingress / TLS]
  end
  subgraph satark[namespace: satark]
    bot[telegram-bot<br/>Go :8082]
    web[pwa<br/>nginx :8080]
    gw[gateway-api<br/>Go :8080]
    txt[text-analyzer<br/>Python :8000]
    qr[qr-decoder<br/>Python :8001]
    url[url-intel<br/>Go :8081]
    rep[reputation<br/>Go :8083]
    op[linkscan-operator]
    ops[opsbot :8002]
  end
  subgraph sandbox[namespace: satark-sandbox · gVisor]
    sb[sandbox-browser pods<br/>one per link]
  end
  subgraph data[namespace: satark-data]
    pg[(Postgres satark-pg)]
    vk[(Valkey)]
    nats[(NATS)]
  end
  ing --> web & gw & bot
  bot --> gw
  bot --> rep
  gw --> txt & qr & url & rep
  gw --> pg & vk
  url --> op --> sb
  rep --> pg
  url --> nats
```

| Service | Responsibility | State |
|---|---|---|
| `gateway-api` | Public API, API keys, per-key rate limits, fan-out, risk scoring | `api_keys` in Postgres, counters in Valkey |
| `telegram-bot` | Conversation, language, report flow, `/forget` | per-user settings |
| `text-analyzer` | PII redaction, rules (`rules/{en,hi,kn}.yaml`), LLM classification | none |
| `qr-decoder` | QR decoding, UPI deep-link parsing | none |
| `url-intel` | Redirects, DNS/RDAP/TLS, lookalikes, feeds | feed cache |
| `reputation` | Community reports, thresholds, delisting | `reputation_*` tables |
| `linkscan-operator` + `sandbox-browser` | Open a link in a throwaway, isolated pod | screenshots (7 days) |
| `pwa` | Web front end and Android share target | none server-side |

## 3. Checking a message

```mermaid
sequenceDiagram
  autonumber
  participant U as User
  participant B as telegram-bot / PWA
  participant G as gateway-api
  participant T as text-analyzer
  participant L as url-intel
  participant R as reputation
  participant M as LLM
  U->>B: forwards message
  B->>G: POST /v1/check/text
  par analyse
    G->>T: text
    T->>T: redact PII, run rules
    T->>M: redacted text only
    M-->>T: classification + explanation
  and links
    G->>L: extracted URLs
    L-->>G: domain age, TLS, lookalike, feeds
  and community
    G->>R: GET /v1/lookup (numbers, UPI IDs, domains)
    R-->>G: thresholded, masked summary
  end
  G->>G: combine into risk + reasons
  G-->>B: {risk, reasons, advice}
  B-->>U: plain-language verdict (never "100% safe")
```

Timeouts are per branch: a slow LLM or sandbox degrades the answer (fewer reasons) rather than failing it.

## 4. Community reports

```mermaid
flowchart LR
  rpt[POST /v1/reports] --> norm[normalise<br/>+91 / www. / case]
  norm --> hash[HMAC with pepper<br/>subject + reporter]
  hash --> rl{per-reporter<br/>rate limit}
  rl -- ok --> db[(reputation_reports<br/>unique per reporter+subject)]
  rl -- exceeded --> r429[429 Retry-After]
  lk[GET /v1/lookup] --> agg[count distinct reporters]
  db --> agg
  agg --> th{>= 3?}
  th -- no --> none[insufficient_reports]
  th -- yes --> out[masked subject · bucketed count<br/>categories that pass threshold · month]
```

- No raw phone numbers, VPAs or user IDs are stored. Without the pepper (a Kubernetes secret), hashes can't be linked back.
- A single person can't make something appear: the threshold counts distinct reporters, and each category must pass it separately.
- Owners can ask for review (SOP-07). Approval deletes the reports.
- `/forget` deletes a reporter's reports; retention deletes everything after 365 days.

## 5. Link sandboxing

```mermaid
sequenceDiagram
  participant L as url-intel
  participant K as Kubernetes API
  participant O as linkscan-operator
  participant S as sandbox pod (gVisor)
  L->>K: create LinkScan CR
  K-->>O: watch event
  O->>K: create Pod (runtimeClass gvisor, no SA token, egress-only NetworkPolicy)
  S->>S: load page headless, follow redirects, screenshot, detect forms
  Note over S: never submits forms or types data
  S-->>O: result
  O->>K: update LinkScan status, delete Pod
  K-->>L: status
```

## 6. Data and privacy

See [privacy policy](privacy-policy.md) and [data retention](compliance/data-retention.md). In short: check inputs aren't stored; PII is redacted before any LLM call; reports are hashed, thresholded and masked; `/forget` and a daily retention CronJob delete data.

## 7. Platform

```mermaid
flowchart LR
  dev[git push] --> ci[GitHub Actions<br/>test · scan · build · sign]
  ci --> reg[(GHCR)]
  ci --> gitops[gitops/envs/dev,prod]
  gitops --> argo[Argo CD] --> cluster[kubeadm cluster]
  cluster --> prom[Prometheus · Loki · Grafana]
  prom --> am[Alertmanager] --> opsbot[opsbot<br/>runbook-guided diagnosis]
```

- **Infrastructure:** Terraform (`infra/terraform`) and Ansible (`infra/ansible`) build a kubeadm cluster; Vagrant gives a local copy.
- **Delivery:** every service ships a Helm chart on `charts/satark-common`; Argo CD syncs `gitops/envs/*`.
- **Policy:** Kyverno (`policies/kyverno`) enforces non-root, signed images and resource limits; Checkov scans IaC.
- **Observability:** every service exposes `/healthz`, `/readyz` and `/metrics` with the `satark_` prefix.
- **Operations:** runbooks in `docs/runbooks`, procedures in `docs/sops`, decisions in `docs/adr`.
