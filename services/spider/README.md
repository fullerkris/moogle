# Spider

The spider crawls the web using breadth-first search (BFS), extracts links and images,
and stores crawl output in Redis for downstream services.

Recent improvements in this service include:
- Redis-backed URL deduplication and visited tracking
- Transactional page persistence plus indexer queue publishing
- Hardened HTTP fetching (timeouts, user-agent, response-size limits)
- Deterministic tests with `httptest` and `miniredis`
- Heuristic frontier scoring (crawler-first) with near-BFS depth ordering

## Setup

### Environment variables

Create a `variables.env` file in `services/spider`.

```env
REDIS_HOST=<your_redis_host>
REDIS_PORT=<your_redis_port>                # default: 6379
REDIS_PASSWORD=<your_redis_password>        # default: empty
REDIS_DB=<your_redis_db>                    # default: 0
STARTING_URL=<your_starting_url>            # default: https://en.wikipedia.org/wiki/Kamen_Rider

SPIDER_HTTP_TIMEOUT_SECONDS=<seconds>       # default: 10
SPIDER_HTTP_MAX_BODY_BYTES=<max_bytes>      # default: 2097152 (2 MiB)
SPIDER_HTTP_USER_AGENT=<crawler_user_agent> # default: MoogleSpider/1.0 (+https://github.com/IonelPopJara/search-engine)
```

### Using Docker

Using Docker is the recommended way to run the spider.

1. Install Docker for your OS from [Docker Docs](https://docs.docker.com/get-docker/).
2. From `services/spider`, build and start:

```bash
docker compose up --build
```

Run in detached mode:

```bash
docker compose up --build -d
```

Scale spider workers (service name from `docker-compose.yml` is `spider-service`):

```bash
docker compose up --build -d --scale spider-service=3
```

Stop:

```bash
docker compose down
```

### Without Docker

You can run the spider locally with Go.

1. Install Go (Go 1.23+ recommended based on `go.mod`).
2. Export env vars:

```bash
export $(grep -v '^#' variables.env | xargs)
```

3. Build:

```bash
go build -o spider ./cmd/spider
```

4. Run:

```bash
./spider -max-concurrency=10 -max-pages=100
```

For development, you can run directly:

```bash
go run ./cmd/spider -max-concurrency=10 -max-pages=100
```

## Crawl state and Redis keys

The spider tracks URL lifecycle in Redis with separate keys for dedupe and completion.

| Key | Type | Purpose |
|---|---|---|
| `spider_queue` | ZSET | Crawl frontier ordered by score/depth |
| `spider_seen_urls` | SET | Global dedupe for normalized URLs |
| `spider_visited_urls` | SET | URLs successfully crawled |
| `normalized_url:<normalized_url>` | HASH | URL lookup metadata (`raw_url`, `visited`) |
| `page_data:<normalized_url>` | HASH | Serialized page data consumed by indexer |
| `pages_queue` | LIST | Queue of page keys consumed by indexer |

## Frontier scoring (crawler-first)

The spider still uses `spider_queue` as a Redis `ZSET`, where lower score means higher priority (`BZPopMin`).

Scoring is now:

`next_depth + fractional_penalty`

- `next_depth` is computed as `floor(parent_score) + 1` to preserve near-BFS behavior across layers.
- `fractional_penalty` is heuristic-only (no static whitelist/blacklist):
  - domain/TLD quality signals (`.edu`, `.gov`, `.org`, noisy/low-quality TLDs, host complexity)
  - URL shape signals (query presence, deep paths, noisy host tokens)
  - deterministic tie-breaker hash for stable ordering inside the same depth

This is additive and Redis-safe: key names/contracts for downstream indexers are unchanged.

### URL lifecycle

1. `PushURL(rawURL, score)` strips and normalizes the URL.
2. A Redis Lua script atomically:
   - checks membership in `spider_seen_urls`
   - stores URL lookup metadata (`raw_url`, `visited=0`)
   - enqueues unseen normalized URL in `spider_queue`
3. Worker pops from `spider_queue`, fetches/parses, and on success marks visited:
   - adds normalized URL to `spider_visited_urls`
   - updates `normalized_url:*` hash (`visited=1`)

## Persistence and queue consistency

Page persistence is batched in a Redis transaction pipeline. For each page, spider writes
`page_data:<normalized_url>` and publishes that key to `pages_queue` in the same transaction,
which reduces queue/data drift risk.

## Fetch behavior and hardening

For each URL, the spider uses a configured HTTP client with protective limits:

- Request timeout from `SPIDER_HTTP_TIMEOUT_SECONDS`
- `User-Agent` header from `SPIDER_HTTP_USER_AGENT`
- Content-type gate for HTML responses only (`text/html`)
- Max response body size from `SPIDER_HTTP_MAX_BODY_BYTES`
- Non-2xx/3xx responses are treated as fetch errors and skipped

## Testing

Run all spider tests from `services/spider`:

```bash
go test ./...
```

Run focused suites:

```bash
go test ./internal/database -run "TestPushURLDedupAndRawMapping|TestVisitPageAndSeenDedup" -v
go test ./internal/controllers -run TestSavePagesWritesPageDataAndIndexerQueue -v
go test ./internal/crawler -run TestGetPageData -v
go test ./internal/crawler -run TestComputeFrontierScore -v
```

Testing strategy:
- `httptest` for deterministic HTTP fetch behavior (timeouts, status, content-type, size limit)
- `miniredis` for deterministic Redis behavior (dedupe, visited markers, queue contract)

## Notes

- Make sure Redis is running and reachable with the configured credentials.
- If dependencies are missing locally, run `go mod tidy`.
- Current dedupe behavior is strict for seen URLs; recrawl scheduling/TTL is a future enhancement.
