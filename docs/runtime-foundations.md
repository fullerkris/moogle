# Runtime Foundations (VM + Kubernetes)

This document introduces the first deployable runtime baseline for both target platforms.

## VM Baseline

- Compose profile: `deploy/compose/docker-compose.prod.yml`
- Ingress: Caddy with a hardened baseline in `deploy/compose/Caddyfile`
- Public exposure: Caddy only (`:80`)
- Internal services: query-engine app runtime, isolated query Redis, isolated pipeline Redis, MongoDB

Run locally:

```bash
docker compose -f deploy/compose/docker-compose.prod.yml up -d
```

## Kubernetes Baseline (Kustomize)

- Base manifests in `k8s/base`
- Overlays in:
  - `k8s/overlays/dev`
  - `k8s/overlays/staging`
  - `k8s/overlays/prod`

Apply dev overlay:

```bash
kustomize build k8s/overlays/dev | kubectl apply -f -
```

## Included Runtime Components

- `query-engine` deployment (Laravel/PHP-FPM)
- `query-engine-caddy` deployment (HTTP ingress inside cluster)
- `query-redis` deployment/service
- `pipeline-redis` deployment/service
- `Ingress` with default timeout/body-size controls

## Platform Parity Contract

- Shared environment contract via explicit Redis URLs:
  - `PIPELINE_REDIS_URL`
  - `QUERY_REDIS_URL`
  - `CACHE_REDIS_URL`
- Ingress remains the only public entrypoint.
- Query and pipeline Redis roles stay isolated across both platforms.

## Follow-up Items

- Integrate Vault secret references into Kubernetes overlays.
- Add MongoDB operator/managed database strategy for HA.
- Wire TLS cert management (cert-manager) for production ingress.
