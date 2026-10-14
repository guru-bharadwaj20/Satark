# ADR-002: Gateway API (Envoy Gateway) instead of ingress-nginx

- **Status:** Accepted
- **Date:** 2026-10-14
- **Deciders:** guru-bharadwaj20

## Context

Satark exposes two things publicly: the REST API (`gateway-api`) and the
Telegram webhook (`telegram-bot`). Traffic arrives from the AWS edge proxy
over WireGuard and needs TLS termination, HTTP-to-HTTPS redirects, per-route
timeouts, and later rate limiting and header-based routing. Two environments
(`satark-dev`, `satark`) share one cluster, and the routes should be owned by
the service charts, not by a central ingress file.

The usual default is the Kubernetes `Ingress` API with ingress-nginx.

## Decision

Use the **Gateway API** with **Envoy Gateway** as the implementation.

- One `GatewayClass` (`satark`) and one `Gateway` (`satark-public`) owned by
  the platform in `envoy-gateway-system`.
- Each service chart ships its own `HTTPRoute` through the `satark-common`
  library chart. The Gateway only accepts routes from the `satark` and
  `satark-dev` namespaces.
- CRDs are pinned (Gateway API v1.2.1, standard channel) and applied before
  the controller.

## Alternatives considered

| Option | Why not |
|---|---|
| ingress-nginx | `Ingress` has no role split between platform and app teams; timeouts, redirects and rate limits are controller-specific annotations; the project entered maintenance mode and the Ingress API is frozen. |
| Traefik | Good Gateway API support, but its richer features are CRDs of its own, and Envoy is the data plane used by most service meshes, which is more transferable knowledge. |
| Cilium Gateway API | Avoids another component since Cilium is the CNI (ADR-003), but couples ingress upgrades to CNI upgrades and has fewer traffic-policy features today. |
| Istio ingress gateway | A full mesh is more than the project needs. |

## Consequences

**Positive**

- Clear ownership: platform owns the Gateway and TLS, services own routes.
- Portable, typed config (timeouts, redirects, header matches) instead of
  annotations; switching implementation later is mostly a GatewayClass change.
- Envoy's `BackendTrafficPolicy` / `SecurityPolicy` give global rate limiting
  and JWT/API-key auth at the edge when needed.

**Negative**

- More CRDs to manage and upgrade; CRD versions must be pinned and applied
  first (sync wave 0 in GitOps).
- Fewer copy-paste examples online than for ingress-nginx.
- Envoy Gateway runs a controller plus per-Gateway Envoy pods, slightly more
  resources than a single ingress controller.
