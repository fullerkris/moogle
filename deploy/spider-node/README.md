# Spider Node

This deployment target runs the existing spider service as a standalone crawl node on a
separate machine.

The node only needs one shared dependency:

- `PIPELINE_REDIS_URL`

Everything else stays centralized. The node writes the same Redis queue/data contracts that the
main `indexer`, `image-indexer`, and `backlinks-processor` already consume.

## Modes

### Local prototype

Use the local override file to spin up a bundled Redis broker alongside the spider node.

1. Copy the env file:

```bash
cp deploy/spider-node/.env.example deploy/spider-node/.env
```

2. Start the prototype:

```bash
docker compose -f deploy/spider-node/docker-compose.yml -f deploy/spider-node/docker-compose.local.yml up --build -d
```

3. Check readiness:

```bash
curl http://127.0.0.1:22113/health/ready
```

4. Stop it:

```bash
docker compose -f deploy/spider-node/docker-compose.yml -f deploy/spider-node/docker-compose.local.yml down -v
```

### Remote node

For a spider node on another VM, run only `deploy/spider-node/docker-compose.yml` and set
`PIPELINE_REDIS_URL` to a Redis endpoint the VM can reach over a private path.

Example:

```env
PIPELINE_REDIS_URL=redis://:replace-me@100.99.200.105:16379/0
SPIDER_STARTING_URL=https://www.reddit.com/
SPIDER_CRAWL_MODE=allowlist
SPIDER_ALLOWLIST_DOMAINS=reddit.com
```

On the main server, the matching production settings are:

```env
PIPELINE_REDIS_PASSWORD=replace-me
PIPELINE_REDIS_PRIVATE_BIND_IP=100.99.200.105
PIPELINE_REDIS_PRIVATE_PORT=16379
```

Then start it with:

```bash
docker compose -f deploy/spider-node/docker-compose.yml up --build -d
```

## Private connectivity requirement

The current production `pipeline-redis` in `deploy/compose/docker-compose.prod.yml` is only on
the internal Docker bridge network. A remote spider node cannot reach it until you provide a
private path, for example:

1. Publish Redis on a private interface or Tailscale address.
2. Put a TCP proxy in front of Redis on a private network.
3. Use an SSH or Tailscale tunnel from the spider node to the main VM.

This deployment target intentionally does not assume which of those you want.

## Proxy support

The spider already honors Go's standard proxy env vars:

- `HTTP_PROXY`
- `HTTPS_PROXY`
- `NO_PROXY`

That lets you move crawl egress off the host VM without changing spider code.
