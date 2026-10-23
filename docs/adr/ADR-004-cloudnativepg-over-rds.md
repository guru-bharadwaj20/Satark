# ADR-004: CloudNativePG on-prem instead of managed RDS

- **Status:** Accepted
- **Date:** 2026-10-14
- **Deciders:** guru-bharadwaj20

## Context

Satark stores scan results, cached link intelligence, community reports and
feedback in PostgreSQL, and will add pgvector for similarity search in the
GenAI phase. All application services run on the on-prem kubeadm cluster;
AWS is used for the edge proxy, DNS and S3 (backups, WAL, screenshots).

The database needs: high availability across nodes, point-in-time recovery,
low latency from `gateway-api`, pgvector, and a cost that fits a personal
project. It must also be operable through GitOps like everything else.

## Decision

Run PostgreSQL **in the cluster with the CloudNativePG (CNPG) operator**:

- Cluster `satark-pg` in `satark-data`: three instances (one primary, two
  replicas), synchronous replication to one replica, pod anti-affinity by
  host, separate WAL volume.
- Schema changes via golang-migrate run as a Kubernetes Job.
- Backups: CNPG's Barman Cloud integration streams WAL and base
  backups to S3, giving PITR without managing Barman separately.

## Alternatives considered

| Option | Why not |
|---|---|
| Amazon RDS / Aurora | Every query would cross the WireGuard tunnel from on-prem to AWS (tens of ms per round trip, plus a single tunnel as a dependency for every scan). A Multi-AZ instance costs more per month than the whole rest of the project. |
| Self-managed Patroni on VMs | Proven, but needs its own etcd/Consul, VM lifecycle and config management outside Kubernetes and GitOps. |
| Zalando / Crunchy operators | Capable, but CNPG has no external HA dependency (uses the Kubernetes API for leader election), is a CNCF project, and its declarative Cluster/Backup/ScheduledBackup CRDs fit Argo CD well. |
| Single Postgres StatefulSet | No automated failover, no PITR. |

## Consequences

**Positive**

- Sub-millisecond latency from the app pods; no cloud dependency on the hot path.
- Failover, switchover for minor upgrades, and replica rebuilds handled by the operator.
- Declarative config in Git; the same manifests work on any conformant cluster.
- pgvector available from the CNPG images.

**Negative**

- The team owns database operations: storage capacity, upgrades, restore
  drills (scheduled game days), and monitoring.
- `local-path` storage ties each instance to a node; losing a node means
  rebuilding that replica from the primary. Acceptable with three instances,
  but a shared/replicated StorageClass would be needed for larger data sets.
- A full-cluster loss depends on the S3 backups being tested regularly.

## Revisit when

- Data volume or availability requirements exceed what three local-disk nodes
  can provide, or
- application services move to AWS, which removes the latency argument for
  keeping the database on-prem.
