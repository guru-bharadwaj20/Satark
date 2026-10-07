# Contributing to Satark

Thanks for your interest in Satark. This document explains how to propose changes, the conventions the project follows, and what a pull request needs before it can be merged. To see what's done and what's next, jump to the [project timeline](#project-timeline).

## Ways to contribute

- **Report a bug** or an inaccurate verdict by opening an issue.
- **Share scam samples.** New scam messages, links or QR codes help improve detection. Remove all personal data first (your number, name, account details).
- **Improve detection rules** in `rules/`. These are YAML, so you don't need to know Go or Python.
- **Add or improve translations.** Hindi, Kannada and other Indian languages are all welcome.
- **Write code**: services, Helm charts, Ansible roles, Terraform modules, policies, tests.
- **Improve documentation**: runbooks, SOPs, ADRs and guides.

## Workflow

1. **Open or pick an issue** before starting anything non-trivial, so work isn't duplicated.
2. **Fork** the repository (or create a branch if you have write access).
3. **Name your branch** using one of these prefixes:

   | Prefix | Use for |
   |---|---|
   | `feat/` | New features |
   | `fix/` | Bug fixes |
   | `infra/` | Ansible, Terraform, Helm, GitOps changes |
   | `docs/` | Documentation only |
   | `test/` | Tests only |
   | `chore/` | Tooling, dependencies, housekeeping |

   Example: `feat/qr-decoder-upi-parsing`

4. **Make small, focused commits** (see the commit conventions below).
5. **Open a pull request** against `main` and fill in the template.
6. **Address review comments.** At least one approving review is required before merging.

## Commit conventions

The project follows [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short summary>

<optional body explaining what and why>
```

- **Types:** `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `ci`, `build`, `chore`
- **Scope:** the service or area, e.g. `gateway-api`, `ansible`, `terraform`, `rules`
- Write the summary in the imperative ("add UPI parsing", not "added UPI parsing").

Example:

```
feat(qr-decoder): flag UPI QRs presented as "receive money"
```

## Code style

| Language / tool | Standard |
|---|---|
| Go | `gofmt`, `golangci-lint`; `context` timeouts on all outbound calls; structured logging with `slog` |
| Python | `ruff` (lint and format), type hints, `pytest` |
| TypeScript | ESLint + Prettier |
| Terraform | `terraform fmt`, `tflint`; every resource tagged with `Project` and `Owner`; secure defaults in modules |
| Ansible | `ansible-lint`; idempotent tasks; variables documented in each role's `defaults/main.yml` |
| Helm | Build on the `satark-common` library chart; probes, resource limits and a PDB for every service |
| Shell | `shellcheck`; `set -euo pipefail` |

## Pull request checklist

Before you request a review:

- [ ] Tests are added or updated, and they pass locally
- [ ] Linters pass
- [ ] No secrets, tokens or personal data in the diff
- [ ] Documentation is updated (README, runbook, SOP or ADR as needed)
- [ ] For infrastructure changes: `terraform plan` output or a description of the cluster impact is in the PR
- [ ] For detection changes: the evaluation in `eval/` has been run and the results are in the PR

## Documentation standards

- **ADRs** (`docs/adr/`) record significant design decisions: context, decision, alternatives considered, consequences.
- **Runbooks** (`docs/runbooks/`) follow this template: Symptoms → Impact → Diagnosis → Fix → Verify → Escalation → Prevention.
- **SOPs** (`docs/sops/`) are step-by-step procedures with copy-paste commands and a verification step at the end.

## Privacy and safety rules

These apply to every contribution:

- Never commit real personal data. Mask phone numbers (`98XXXXXX21`) and UPI IDs (`xy***@examplebank`) in examples and test fixtures.
- Personal data must be redacted before any text is sent to an LLM.
- The sandbox must never submit forms or enter data into a page.
- User-facing text must never call something "100% safe" or call a person a "scammer".

## Reporting security issues

Please **don't** open a public issue for security vulnerabilities. Contact the maintainer privately through GitHub instead, with steps to reproduce. You'll get an acknowledgement as soon as possible.

## Code of conduct

Be respectful and constructive. Critique ideas, not people. Harassment or discrimination of any kind isn't tolerated.

## Project timeline

The project is split into 13 phases. Each subtask is marked ✅ (completed) or ❌ (not completed yet). Want to help? Pick a ❌ task, open an issue that references its number (for example `Task 2.12`), and say you're working on it.

**Overall progress: 12 / 294 subtasks completed**

| Phase | Focus | Subtasks | Completed |
|---|---|---|---|
| 0 | Planning and repository setup | 21 | 12 |
| 1 | On-prem Kubernetes foundations | 25 | 0 |
| 2 | Product MVP (core services) | 26 | 0 |
| 3 | Data layer and Helm packaging | 20 | 0 |
| 4 | GitOps and CI/CD | 19 | 0 |
| 5 | Cloud infrastructure with Terraform | 24 | 0 |
| 6 | Observability | 20 | 0 |
| 7 | Link intelligence | 19 | 0 |
| 8 | Sandbox operator | 25 | 0 |
| 9 | Security and compliance | 20 | 0 |
| 10 | GenAI | 25 | 0 |
| 11 | Reliability, backups and disaster recovery | 31 | 0 |
| 12 | Polish, documentation and launch | 19 | 0 |
| | **Total** | **294** | **12** |

Phases 0–4 produce a working MVP: a Telegram bot that checks messages and QR codes, running on a GitOps-managed on-prem cluster.

### Phase 0: Planning and repository setup

- ✅ **0.1** Identify the problem: everyday UPI and digital payment scams in India
- ✅ **0.2** Define target personas (elderly parents, shop owners, students, marketplace sellers)
- ✅ **0.3** Write the project proposal and high-level design
- ✅ **0.4** Choose the project name (Satark)
- ✅ **0.5** Draft the system architecture (product layer and platform layer)
- ✅ **0.6** Choose the tech stack (Go, Python, kubeadm, Terraform, Ansible, Argo CD and others)
- ✅ **0.7** Plan the roadmap
- ✅ **0.8** Create the GitHub repository
- ✅ **0.9** Write the README
- ✅ **0.10** Write the contribution guidelines
- ✅ **0.11** Add the MIT license
- ✅ **0.12** Break the roadmap into phases and subtasks
- ❌ **0.13** Set up branch protection on `main`
- ❌ **0.14** Create a GitHub Projects board (Backlog, Sprint, In Progress, Review, Done)
- ❌ **0.15** Add issue templates (bug, feature request, scam sample, runbook)
- ❌ **0.16** Add a pull request template
- ❌ **0.17** Add `.gitignore` and `.editorconfig`
- ❌ **0.18** Add a pre-commit config (gitleaks, formatters, linters)
- ❌ **0.19** Create the `docs/` skeleton (adr, runbooks, sops, game-days, sprints, compliance)
- ❌ **0.20** Write `SECURITY.md` with the vulnerability disclosure policy
- ❌ **0.21** Write the ADR template and the runbook template

### Phase 1: On-prem Kubernetes foundations

- ❌ **1.1** Write `scripts/setup-host.ps1` to install VirtualBox, Vagrant, Git and WSL2 through winget
- ❌ **1.2** Set up WSL2 Ubuntu as the Ansible control node
- ❌ **1.3** Write the Vagrantfile for `cp-1`, `worker-1` and `worker-2` on `192.168.56.0/24`
- ❌ **1.4** Create the Ansible inventory for on-prem nodes
- ❌ **1.5** Role `common`: base packages, users, chrony time sync, journald limits
- ❌ **1.6** Role `common`: unattended security upgrades
- ❌ **1.7** Role `hardening`: SSH key-only login, no root login, fail2ban
- ❌ **1.8** Role `hardening`: nftables default-deny with per-node-type port allowlists
- ❌ **1.9** Role `hardening`: kernel sysctl hardening
- ❌ **1.10** Role `k8s_prereqs`: disable swap, load `br_netfilter` and `overlay`, enable IP forwarding
- ❌ **1.11** Role `k8s_prereqs`: pkgs.k8s.io apt repository pinned to the N-1 minor version
- ❌ **1.12** Role `containerd`: install and configure `SystemdCgroup = true`
- ❌ **1.13** Role `kubeadm_cp`: templated kubeadm config with API audit logging enabled
- ❌ **1.14** Role `kubeadm_cp`: `kubeadm init` and kubeconfig distribution
- ❌ **1.15** Role `kubeadm_worker`: join token generation and `kubeadm join`
- ❌ **1.16** Role `cilium`: install the Cilium CNI with Hubble enabled
- ❌ **1.17** Install MetalLB in L2 mode with an address pool
- ❌ **1.18** Install metrics-server
- ❌ **1.19** Install local-path-provisioner as the default StorageClass
- ❌ **1.20** Write `playbooks/site.yml` to run all roles in order
- ❌ **1.21** Write `scripts/bootstrap.sh` (`vagrant up` followed by `site.yml`)
- ❌ **1.22** Verify the cluster: all nodes Ready, in-cluster DNS works, a test LoadBalancer gets an IP
- ❌ **1.23** ADR-001: kubeadm instead of k3s
- ❌ **1.24** ADR-003: Cilium as the CNI
- ❌ **1.25** SOP-06: onboarding, from zero to a running cluster

### Phase 2: Product MVP (core services)

- ❌ **2.1** Define scan request and response schemas (first OpenAPI draft)
- ❌ **2.2** `text-analyzer`: FastAPI skeleton with health and metrics endpoints
- ❌ **2.3** `text-analyzer`: redact phone numbers
- ❌ **2.4** `text-analyzer`: redact card numbers using the Luhn check
- ❌ **2.5** `text-analyzer`: redact Aadhaar numbers using the Verhoeff check
- ❌ **2.6** `text-analyzer`: redact PAN, email addresses, account numbers and UPI IDs
- ❌ **2.7** `text-analyzer`: rule engine that loads YAML rules
- ❌ **2.8** English rule set (urgency, KYC, utility, remote access, OTP/PIN, lottery, task jobs)
- ❌ **2.9** Hindi and Kannada rule sets
- ❌ **2.10** Unit tests for redaction and rules
- ❌ **2.11** `qr-decoder`: decode QR codes from images with zxing-cpp
- ❌ **2.12** `qr-decoder`: parse UPI deep links (`pa`, `pn`, `am`, `cu`, `tn`)
- ❌ **2.13** `qr-decoder`: detect the "scan to receive money" pattern
- ❌ **2.14** `qr-decoder`: unit tests with sample QR images
- ❌ **2.15** `gateway-api`: Go skeleton with router and `slog` logging
- ❌ **2.16** `gateway-api`: `/v1/scan/text` and `/v1/scan/qr` endpoints
- ❌ **2.17** `gateway-api`: fan-out to analyzers with `context` timeouts
- ❌ **2.18** `gateway-api`: scoring engine and verdict buckets, with unit tests
- ❌ **2.19** `telegram-bot`: long-polling setup
- ❌ **2.20** `telegram-bot`: handle forwarded text messages
- ❌ **2.21** `telegram-bot`: handle photos (QR screenshots)
- ❌ **2.22** `telegram-bot`: language selection (English, Hindi, Kannada)
- ❌ **2.23** `telegram-bot`: "Who sent this?" follow-up buttons
- ❌ **2.24** `telegram-bot`: reply templates with reasons, next steps and feedback buttons
- ❌ **2.25** Multi-stage Dockerfiles for all MVP services (non-root, minimal base images)
- ❌ **2.26** Docker Compose setup for local development

### Phase 3: Data layer and Helm packaging

- ❌ **3.1** Install the CloudNativePG operator
- ❌ **3.2** Create a three-instance PostgreSQL cluster
- ❌ **3.3** Write SQL migrations (scans, url_intel, entities, reports, feedback)
- ❌ **3.4** Run migrations as a Kubernetes Job
- ❌ **3.5** HMAC-keyed hashing for inputs and reported entities
- ❌ **3.6** `gateway-api`: persist scan results
- ❌ **3.7** Deploy Valkey
- ❌ **3.8** Verdict cache in Valkey
- ❌ **3.9** Token-bucket rate limiting in Valkey
- ❌ **3.10** Helm library chart `satark-common`: Deployment template
- ❌ **3.11** Library chart: Service, HPA and PDB templates
- ❌ **3.12** Library chart: ServiceMonitor and NetworkPolicy templates
- ❌ **3.13** Library chart: restricted security context by default
- ❌ **3.14** Per-service charts for the MVP services
- ❌ **3.15** Install Envoy Gateway and the Gateway API CRDs
- ❌ **3.16** Gateway and HTTPRoutes for the API and the bot
- ❌ **3.17** Probes, resource requests and resource limits for every service
- ❌ **3.18** Deploy the MVP to the cluster with Helm and run smoke tests
- ❌ **3.19** ADR-002: Gateway API instead of ingress-nginx
- ❌ **3.20** ADR-004: CloudNativePG on-prem instead of managed RDS

### Phase 4: GitOps and CI/CD

- ❌ **4.1** Role `argocd_bootstrap`: install Argo CD
- ❌ **4.2** `gitops/bootstrap` app-of-apps root
- ❌ **4.3** ApplicationSet that creates one app per service per environment
- ❌ **4.4** Sync waves for CRDs, platform, data and apps
- ❌ **4.5** `dev` and `prod` environment folders with value overrides
- ❌ **4.6** Argo CD managing its own installation
- ❌ **4.7** GitHub Actions: Go lint (golangci-lint) and tests
- ❌ **4.8** GitHub Actions: Python lint (ruff) and pytest
- ❌ **4.9** GitHub Actions: `helm lint` and `kubeconform`
- ❌ **4.10** Multi-arch image builds with `docker buildx`
- ❌ **4.11** Push images to GHCR
- ❌ **4.12** Trivy image scan (fail on CRITICAL) and Syft SBOM generation
- ❌ **4.13** cosign keyless image signing
- ❌ **4.14** Automatic image tag bump in `gitops/envs/dev`
- ❌ **4.15** Prod promotion through a PR with required review
- ❌ **4.16** Path filters and layer caching so only changed services rebuild
- ❌ **4.17** Test rollback with `git revert` and record how long it takes
- ❌ **4.18** ADR-007: pull-based GitOps
- ❌ **4.19** SOP-04: promote a release to prod and roll back

### Phase 5: Cloud infrastructure with Terraform

- ❌ **5.1** Set up the AWS account (MFA on root, separate admin identity)
- ❌ **5.2** Module `budget-alarm`, applied before anything else
- ❌ **5.3** Bootstrap the S3 bucket for Terraform state with `use_lockfile`
- ❌ **5.4** Module `secure-bucket` (encryption, versioning, public access block, TLS-only, lifecycle)
- ❌ **5.5** Create the Velero, Postgres WAL and screenshot buckets
- ❌ **5.6** Module `github-oidc-role`
- ❌ **5.7** Separate read-only (plan) and apply roles for CI
- ❌ **5.8** Module `edge-node`: VPC, public subnet, internet gateway, route table
- ❌ **5.9** Module `edge-node`: security group allowing only 80, 443 and 51820/udp (no port 22)
- ❌ **5.10** Module `edge-node`: Graviton EC2 with IMDSv2, encrypted EBS and an Elastic IP
- ❌ **5.11** Module `edge-node`: SSM agent and Session Manager access
- ❌ **5.12** Module `dns`: Route 53 hosted zone and records
- ❌ **5.13** IAM policy for cert-manager DNS-01, scoped to the hosted zone
- ❌ **5.14** `envs/dev` and `envs/prod` compositions
- ❌ **5.15** Role `wireguard`: tunnel from `worker-1` to the edge with persistent keepalive
- ❌ **5.16** HAProxy SNI passthrough on the edge to the MetalLB IP through the tunnel
- ❌ **5.17** Install cert-manager with a Let's Encrypt DNS-01 issuer
- ❌ **5.18** Switch `telegram-bot` from long polling to webhooks
- ❌ **5.19** CI: `terraform fmt`, `terraform validate`, `tflint`, `trivy config` and `checkov`
- ❌ **5.20** Custom Checkov policies (mandatory tags, no SSH open to `0.0.0.0/0`)
- ❌ **5.21** CI: post `terraform plan` output as a PR comment
- ❌ **5.22** CI: `terraform apply` from a protected environment
- ❌ **5.23** CloudWatch agent on the edge, plus a WireGuard handshake-age metric
- ❌ **5.24** CloudWatch alarm with SNS email when the tunnel is down

### Phase 6: Observability

- ❌ **6.1** Install kube-prometheus-stack
- ❌ **6.2** Install Loki and Grafana Alloy
- ❌ **6.3** Install the Hubble UI
- ❌ **6.4** RED metrics (rate, errors, duration) in all services
- ❌ **6.5** Business metrics (scans by verdict and category, LLM calls, cache hits)
- ❌ **6.6** ServiceMonitors for all services
- ❌ **6.7** Structured JSON logging across all services
- ❌ **6.8** Dashboards: cluster health and node/Linux
- ❌ **6.9** Dashboard: product usage
- ❌ **6.10** Dashboard: scan pipeline
- ❌ **6.11** Dashboard: SLOs and error budget
- ❌ **6.12** Define SLOs and Prometheus recording rules
- ❌ **6.13** Multi-window burn-rate alerts
- ❌ **6.14** Alert rules with `runbook_url` annotations
- ❌ **6.15** Route Alertmanager notifications to a Telegram ops channel
- ❌ **6.16** Dashboards as code, provisioned through GitOps
- ❌ **6.17** RB-001: Pod in CrashLoopBackOff or ImagePullBackOff
- ❌ **6.18** RB-002: Node NotReady
- ❌ **6.19** RB-005: In-cluster DNS failures
- ❌ **6.20** RB-011: High API error rate or fast SLO burn

### Phase 7: Link intelligence

- ❌ **7.1** `url-intel`: Go service skeleton
- ❌ **7.2** URL normalisation and validation
- ❌ **7.3** Short-link expansion with a bounded number of redirects
- ❌ **7.4** DNS checks (A/AAAA, NS, MX)
- ❌ **7.5** Domain age through RDAP, falling back to WHOIS
- ❌ **7.6** TLS certificate inspection
- ❌ **7.7** Certificate Transparency first-seen lookup
- ❌ **7.8** Lookalike detection (edit distance) against a list of bank and brand domains
- ❌ **7.9** Homoglyph and punycode detection
- ❌ **7.10** Raw IP address and APK link detection
- ❌ **7.11** Official allowlist (`.gov.in`, verified bank domains)
- ❌ **7.12** Google Safe Browsing lookup integration
- ❌ **7.13** `feed-sync` CronJob for URLhaus and OpenPhish
- ❌ **7.14** Load threat feeds into Valkey sets and Postgres
- ❌ **7.15** Cache `url-intel` results with a TTL
- ❌ **7.16** `/v1/scan/url` endpoint in `gateway-api`
- ❌ **7.17** Worker pool and per-call timeouts
- ❌ **7.18** Unit tests with recorded network fixtures
- ❌ **7.19** RB-009: Threat-feed sync is stale

### Phase 8: Sandbox operator

- ❌ **8.1** Scaffold `linkscan-operator` with kubebuilder
- ❌ **8.2** Define the `LinkScan` CRD spec and status
- ❌ **8.3** Reconcile loop: create a hardened sandbox pod with owner references
- ❌ **8.4** Timeouts through `activeDeadlineSeconds` and a `TimedOut` phase
- ❌ **8.5** TTL cleanup of finished `LinkScan` objects
- ❌ **8.6** Kubernetes Events on scan failures
- ❌ **8.7** Prometheus metrics for the operator
- ❌ **8.8** Controller tests with envtest
- ❌ **8.9** `sandbox-browser`: Playwright headless Chromium image
- ❌ **8.10** `sandbox-browser`: record the redirect chain and final URL
- ❌ **8.11** `sandbox-browser`: detect password, OTP and card-number form fields
- ❌ **8.12** `sandbox-browser`: detect file downloads such as APKs
- ❌ **8.13** `sandbox-browser`: upload screenshots to S3
- ❌ **8.14** Install NATS JetStream
- ❌ **8.15** `url-intel` publishes scan jobs to NATS
- ❌ **8.16** Sandbox results published back through NATS
- ❌ **8.17** Role `gvisor`: install `runsc` and the containerd runtime handler
- ❌ **8.18** `gvisor` RuntimeClass
- ❌ **8.19** Sandbox NetworkPolicy: deny all ingress; block RFC 1918, cluster CIDRs and `169.254.169.254` egress
- ❌ **8.20** Verify the SSRF block with Hubble
- ❌ **8.21** KEDA NATS JetStream scaler for sandbox capacity
- ❌ **8.22** `telegram-bot` edits its reply when the sandbox result arrives
- ❌ **8.23** ADR-006: sandbox isolation model
- ❌ **8.24** RB-007: Scan queue backlog
- ❌ **8.25** RB-008: Link-scan sandbox failures

### Phase 9: Security and compliance

- ❌ **9.1** Pod Security Admission `restricted` labels on application namespaces
- ❌ **9.2** Install Kyverno
- ❌ **9.3** Kyverno policies: disallow privileged containers and require non-root
- ❌ **9.4** Kyverno policies: require resource limits and disallow `:latest` tags
- ❌ **9.5** Kyverno policy: verify cosign keyless image signatures
- ❌ **9.6** Run `kyverno test` in CI
- ❌ **9.7** Install Trivy Operator
- ❌ **9.8** kube-bench CIS benchmark Job
- ❌ **9.9** Track kube-bench findings as GitHub issues
- ❌ **9.10** Remediate high-severity findings
- ❌ **9.11** Install Sealed Secrets (or SOPS with age)
- ❌ **9.12** Move all secrets to sealed form
- ❌ **9.13** gitleaks in CI and as a pre-commit hook
- ❌ **9.14** Ship Kubernetes audit logs to Loki
- ❌ **9.15** Default-deny NetworkPolicies for all application namespaces
- ❌ **9.16** Bucket-scoped IAM user for Velero and CNPG backups
- ❌ **9.17** Run IAM Access Analyzer and fix its findings
- ❌ **9.18** Weekly compliance snapshot CronJob
- ❌ **9.19** OWASP ZAP baseline scan against the API
- ❌ **9.20** SOP-03: Rotate secrets and credentials

### Phase 10: GenAI

- ❌ **10.1** LLM provider interface
- ❌ **10.2** Ollama provider for local models
- ❌ **10.3** Amazon Bedrock and hosted API providers, plus the `bedrock-access` Terraform module
- ❌ **10.4** Structured-output prompt validated with Pydantic
- ❌ **10.5** Fall back to rules-only answers on LLM errors or timeouts
- ❌ **10.6** Bounded LLM adjustment to the risk score
- ❌ **10.7** Prompt-injection test cases
- ❌ **10.8** Cache LLM responses by the hash of the redacted text
- ❌ **10.9** Multilingual explanations (Hindi, Kannada)
- ❌ **10.10** Grow the evaluation dataset to 300 labelled messages, including legitimate ones
- ❌ **10.11** `eval/run_eval.py` with precision and recall per category
- ❌ **10.12** CI gate that runs the evaluation when prompts or rules change
- ❌ **10.13** LLM usage and token metrics, plus a budget alert
- ❌ **10.14** ADR-005: Bounded LLM influence on the risk score
- ❌ **10.15** `opsbot`: Alertmanager webhook receiver
- ❌ **10.16** `opsbot`: read-only RBAC ServiceAccount with no access to Secrets
- ❌ **10.17** `opsbot`: gather context (events, pod status, recent rollouts, logs from Loki)
- ❌ **10.18** `opsbot`: redact secrets and tokens before calling the LLM
- ❌ **10.19** Embed runbooks into pgvector
- ❌ **10.20** RAG retrieval of the most relevant runbook sections
- ❌ **10.21** Post diagnoses to the ops Telegram channel
- ❌ **10.22** Per-alert cooldown and daily token budget
- ❌ **10.23** `/ask` command with PromQL and Loki tool calls
- ❌ **10.24** `/postmortem` draft generator
- ❌ **10.25** RB-012: LLM provider outage or cost spike

### Phase 11: Reliability, backups and disaster recovery

- ❌ **11.1** CNPG WAL archiving and base backups to S3
- ❌ **11.2** Test point-in-time recovery for Postgres
- ❌ **11.3** Install Velero with the S3 backend
- ❌ **11.4** Daily Velero schedule, including file-system backups of volumes
- ❌ **11.5** Role `etcd_backup`: systemd timer that uploads etcd snapshots to S3
- ❌ **11.6** Role `k8s_upgrade`: switch the apt repository to the next minor version
- ❌ **11.7** Role `k8s_upgrade`: control-plane upgrade steps
- ❌ **11.8** Role `k8s_upgrade`: worker drain, upgrade and uncordon
- ❌ **11.9** `playbooks/upgrade.yml` with `serial: 1`
- ❌ **11.10** Run an N-1 → N upgrade under k6 load and record the error count
- ❌ **11.11** SOP-01: Kubernetes minor-version upgrade
- ❌ **11.12** `add-node.yml` and `remove-node.yml` playbooks
- ❌ **11.13** SOP-02: Add or remove a node
- ❌ **11.14** Terraform for a temporary DR VM running k3s
- ❌ **11.15** DR drill: Velero restore and DNS switch, recording RTO and RPO
- ❌ **11.16** SOP-05: Disaster recovery drill
- ❌ **11.17** k6 load tests; find and fix the first bottleneck
- ❌ **11.18** `kubectl-satark doctor`: nodes, pods, PVCs and deployments
- ❌ **11.19** `kubectl-satark doctor`: certificate expiry, PDBs, Argo CD status and `--json` output
- ❌ **11.20** Game day 1: lose a worker node
- ❌ **11.21** Game day 2: lose the Postgres primary
- ❌ **11.22** Game day 3: break in-cluster DNS
- ❌ **11.23** Game day 4: apply a bad NetworkPolicy
- ❌ **11.24** Game day 5: ship a bad deploy and roll back
- ❌ **11.25** Game day 6: container runtime failure
- ❌ **11.26** Game day 7: disk pressure on a node
- ❌ **11.27** Game day 8: WireGuard tunnel down
- ❌ **11.28** Game day 9: image pull failure
- ❌ **11.29** Game day 10: certificate expiry drill
- ❌ **11.30** Measure time to diagnose with and without `opsbot`
- ❌ **11.31** Runbooks RB-003 (Postgres), RB-004 (certificates), RB-006 (backups) and RB-010 (tunnel)

### Phase 12: Polish, documentation and launch

- ❌ **12.1** `reputation` service: report and lookup endpoints
- ❌ **12.2** Distinct-reporter thresholds and masked output
- ❌ **12.3** Per-reporter rate limiting
- ❌ **12.4** Delisting request flow and SOP-07
- ❌ **12.5** Report flow in the bot with 1930, cybercrime.gov.in and Chakshu details
- ❌ **12.6** PWA: check text, links and QR codes
- ❌ **12.7** PWA: Web Share Target for the Android share menu
- ❌ **12.8** Publish the OpenAPI spec and API docs
- ❌ **12.9** API key issuance for third-party clients
- ❌ **12.10** `/forget` command to delete a user's data
- ❌ **12.11** Data retention CronJob
- ❌ **12.12** Privacy policy and disclaimer
- ❌ **12.13** Architecture document with diagrams
- ❌ **12.14** Update the README with screenshots and a demo GIF
- ❌ **12.15** Record a three-minute demo video
- ❌ **12.16** Write the launch blog post
- ❌ **12.17** Launch to family, college and residents' association groups
- ❌ **12.18** Track weekly active users, scans and the helpful-rating rate
- ❌ **12.19** Publish a monthly anonymised scam-trends report

## License

By contributing, you agree that your contributions are licensed under the project's [MIT License](LICENSE).
