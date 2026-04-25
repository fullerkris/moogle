# Local Staging Rollout Proof (Vault + Production Compose)

Date: 2026-04-25

## Command Sequence

```bash
VAULT_ADDR=http://127.0.0.1:8200 \
VAULT_TOKEN=<root-token> \
./scripts/vault/run-prod-compose.sh staging up -d

VAULT_ADDR=http://127.0.0.1:8200 \
VAULT_TOKEN=<root-token> \
./scripts/vault/run-prod-compose.sh staging ps

curl -s http://localhost/api/health/live
curl -s http://localhost/api/health/ready
curl -sI http://localhost/metrics
```

## Evidence Snapshot

- All production-compose services reached `Up ... (healthy)` status:
  - `caddy`, `laravel`, `mongo`, `query-redis`, `pipeline-redis`
  - `spider`, `indexer`, `image-indexer`, `backlinks-processor`
- Query-engine liveness and readiness endpoints returned HTTP 200.
- `/metrics` returned HTTP 200 with Prometheus text format (`Content-Type: text/plain; version=0.0.4; charset=utf-8`).
- Root path returned HTTP 302 and redirected to `https://moogle.app`.

## Notes

- Shared Vault URLs use `*.internal` hostnames.
- Production compose now provides matching aliases (`pipeline-redis.internal`, `query-redis.internal`, `mongo.internal`) so the same secret contract resolves in local VM rollout.
- Local Vault bootstrap now seeds a valid Laravel `APP_KEY` placeholder so web middleware routes (including `/metrics`) do not fail with runtime encryption key errors.
