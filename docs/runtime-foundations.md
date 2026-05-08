# Runtime Foundations (VM + Kubernetes)

This document introduces the first deployable runtime baseline for both target platforms.

## VM Baseline

- Compose profile: `deploy/compose/docker-compose.prod.yml`
- Ingress: Caddy with a hardened baseline in `deploy/compose/Caddyfile`
- Public exposure: Caddy only (`:80` for local baseline; `80/443` in production with TLS termination)
- Internal services: query-engine app runtime, spider/indexer/image-indexer/backlinks/tfidf/page-rank workers, isolated query Redis, isolated pipeline Redis, MongoDB

Run locally:

```bash
VAULT_ADDR=http://127.0.0.1:8200 \
VAULT_TOKEN=<vault-token> \
./scripts/vault/run-prod-compose.sh staging up -d
```

Before any staging or production spin-up, validate the live Vault contract:

```bash
VAULT_ADDR=https://vault.example.com \
VAULT_TOKEN=<read-token> \
scripts/ops/validate-secret-readiness.sh prod
```

Notes:

- `run-prod-compose.sh` exports shared + `query-engine` secrets from Vault and enforces required env contracts.
- Compose services expose `*.internal` aliases (`pipeline-redis.internal`, `query-redis.internal`, `mongo.internal`) to match the shared Vault URL contract.
- Spider metrics are exposed on loopback only (`127.0.0.1:2113`) for local scrape without public internet exposure.

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
- `query-engine-metrics` service (internal metrics scrape target for `/metrics`)
- `query-redis` deployment/service
- `pipeline-redis` deployment/service
- `runtime-metrics-exporter` deployment/service (queue and Redis telemetry)
- `Ingress` with default timeout/body-size controls

## Platform Parity Contract

- Shared environment contract via explicit Redis URLs:
  - `PIPELINE_REDIS_URL`
  - `QUERY_REDIS_URL`
  - `CACHE_REDIS_URL`
- Ingress remains the only public entrypoint.
- Query and pipeline Redis roles stay isolated across both platforms.
- Kubernetes metrics scrape hints are attached via `prometheus.io/*` annotations on `query-engine-metrics` and `runtime-metrics` services.

## Follow-up Items

- Add MongoDB operator/managed database strategy for HA.
- Wire TLS cert management (cert-manager) for production ingress.

## Kubernetes Secrets (Vault via External Secrets)

- Base manifests now include:
  - `k8s/base/secretstore-vault.yaml`
  - `k8s/base/externalsecret-runtime-secrets.yaml`
- Runtime deployments consume sensitive values from `moogle-runtime-secrets` (Kubernetes `Secret`) instead of `ConfigMap`.
- Overlays patch Vault paths per environment (`moogle/dev/*`, `moogle/staging/*`, `moogle/prod/*`).

Prerequisites:

- External Secrets Operator must be installed in cluster.
- A Vault token secret must exist in the `moogle` namespace:

```bash
kubectl -n moogle create secret generic vault-token --from-literal=token='<vault-read-token>'
```

Operational runbook:

- `docs/k8s-secret-bootstrap.md`

## Runtime Metrics Image and Scrape Notes

- `runtime-metrics-exporter` image is published to GHCR as `ghcr.io/ionelpopjara/moogle/runtime-metrics-exporter`.
- Kubernetes overlays pin the runtime exporter image reference via each overlay `kustomization.yaml`.
- For Prometheus Operator deployments, apply `k8s/monitoring-operator` to register ServiceMonitor resources and a minimal local `Prometheus` custom resource.
- Follow-up: tune Prometheus retention/storage and resource limits for non-local clusters.

## Project Kubeconfig Template

- Start from `k8s/kubeconfig.template.yaml` and save a local copy as `k8s/kubeconfig.local.yaml`.
- Fill in your cluster API endpoint, CA bundle, and bearer token values.
- Or generate it in one command with:

```bash
scripts/create-kubeconfig-local.sh \
  --server https://YOUR_K8S_API_SERVER:6443 \
  --token "YOUR_BEARER_TOKEN" \
  --ca-file /absolute/path/to/cluster-ca.crt
```

- Run kubectl commands with the project helper script:

```bash
scripts/kubectl-with-config.sh k8s/kubeconfig.local.yaml apply -k k8s/monitoring-operator
```
