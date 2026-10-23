# ADR-007: Pull-based GitOps with Argo CD

- **Status:** Accepted
- **Date:** 2026-10-14
- **Phase:** 4 (GitOps and CI/CD)

## Context

Satark runs on an on-prem kubeadm cluster behind NAT, reached from the
internet only through a WireGuard tunnel to an AWS edge node. We need a way
to deploy a dozen services to `dev` and `prod` that is auditable, easy to roll
back, and does not hand CI the keys to the cluster.

Options considered:

1. **Push-based:** GitHub Actions runs `helm upgrade` / `kubectl apply`
   against the cluster.
2. **Pull-based:** an agent inside the cluster (Argo CD or Flux) watches Git
   and reconciles the cluster to it.

## Decision

Use **pull-based GitOps with Argo CD**.

- Git (`gitops/`) is the single source of truth for what runs where.
- CI builds, scans, signs and pushes images, then only **writes to Git**
  (dev tag bump, prod promotion PR). It has no cluster credentials.
- Argo CD runs in the cluster, polls Git, and applies changes with
  auto-sync, pruning and self-heal. It also manages its own installation.
- An app-of-apps root plus an ApplicationSet create one Application per
  service per environment; sync waves order CRDs, platform, data and apps.

## Consequences

**Good**

- No inbound access or kubeconfig in CI: the cluster stays behind NAT, and a
  leaked CI token cannot change the cluster directly.
- Drift is detected and reverted (self-heal); manual `kubectl edit` does not
  stick.
- Every deploy is a commit, so history is the audit log and rollback is
  `git revert` (measured in the rollback game day).
- Prod changes go through a reviewed PR enforced by CODEOWNERS and branch
  protection.

**Bad**

- Up to one reconciliation interval (180 s) between merge and deploy unless
  a refresh or webhook is sent.
- Argo CD is another critical component to run, upgrade and back up.
- Image tag bumps add bot commits to `main`.
- Secrets cannot live in Git as plain text; we need Sealed Secrets.

## Why Argo CD over Flux

Both fit. Argo CD's UI makes sync state and diffs easy to show in demos and
during incidents, ApplicationSets cover the service-by-environment matrix,
and its health checks and sync waves cover our ordering needs.
