# Security policy

Satark handles messages, links and QR codes that people forward to it, so we take vulnerabilities seriously. Thank you for helping keep users safe.

## Supported versions

Only the latest release and the `main` branch receive security fixes.

## Reporting a vulnerability

**Please don't open a public issue, pull request or discussion for security problems.**

Report privately through GitHub's [private vulnerability reporting](https://github.com/guru-bharadwaj20/satark/security/advisories/new) (Security tab → *Report a vulnerability*). Include:

- the affected component (e.g. `gateway-api`, `telegram-bot`, Helm chart, Terraform module)
- the version or commit
- steps to reproduce or a proof of concept
- the impact you believe it has

Don't include real personal data in your report. Use masked values (`98XXXXXX21`, `xy***@examplebank`).

## What to expect

| Step | Target time |
|---|---|
| Acknowledgement of your report | within 3 working days |
| Initial assessment and severity | within 7 days |
| Fix or mitigation for critical/high issues | within 30 days |
| Public advisory | after a fix is released, coordinated with you |

We'll keep you updated and, if you wish, credit you in the advisory.

## Scope

In scope:

- Code in `apps/`, `tools/`, `charts/`, `gitops/`, `infra/` and `policies/`
- Sandbox escapes, i.e. a scanned page reaching anything outside `satark-sandbox`
- Bypassing PII redaction before data is logged, stored or sent to an LLM
- Authentication, authorisation, injection and SSRF issues in any service
- Secrets or credentials exposed in the repository or published images

Out of scope:

- Detection misses or false positives. These aren't vulnerabilities; please use the *Bug report* or *Scam sample* issue templates.
- Vulnerabilities in third-party dependencies with no demonstrated impact on Satark (report them upstream)
- Denial of service through sheer traffic volume, social engineering, or physical attacks

## Safe harbour

We won't pursue legal action against good-faith research that follows this policy: don't access or modify other people's data, don't degrade the service for others, test only against your own deployment where possible, and give us reasonable time to fix the issue before disclosure.
