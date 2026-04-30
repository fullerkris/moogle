# Moogle Spider Expansion Spec
## Multi-Seed Crawling, Domain Policy, Robots Enforcement, Controlled Bypass, and Frontier Prioritization

## 1. Purpose

The purpose of this spec is to evolve the Moogle spider service from a single-seed crawler into a broader, safer, and more controllable web crawler.

The spider should be able to:

- start from many seeds instead of one
- crawl across multiple domains intentionally
- enforce configurable domain restrictions
- respect robots.txt and crawl-delay by default
- support a controlled bypass list for selected domains
- enforce crawler-defined politeness even when bypass is enabled
- prioritize important seeds and higher-value links
- avoid runaway queue growth
- remain compatible with the current Redis-based crawl frontier

This change is intended to preserve the spirit of the current crawler while giving it the legs to walk beyond a single island.

## 2. Current State

The current spider service behaves as a queue-based crawler with a single bootstrap URL.

Observed behavior today:

- reads a single `STARTING_URL`
- defaults to a Wikipedia page
- pushes that seed into Redis
- pops URLs from the spider queue
- checks whether a URL has already been visited
- fetches page HTML
- extracts outgoing links and images
- stores page, outlink, backlink, and image data
- computes frontier scores for discovered links
- re-enqueues discovered URLs

This means the crawler already contains the beginnings of a real frontier, but it is constrained by a narrow starting point and lacks a robust crawl policy layer.

## 3. Goals

### 3.1 Functional goals

The spider should:

- support multiple starting seeds
- support multiple crawl-policy modes
- enforce allowlist and blocklist rules
- support open-web crawling with constraints
- respect robots.txt by default
- support controlled robots bypass for explicitly approved domains
- enforce politeness windows and per-domain concurrency limits
- prioritize seeds and discovered links using a configurable scoring model
- allow curated crawl campaigns
- preserve compatibility with the current queue-driven architecture

### 3.2 Non-functional goals

The spider should also:

- remain safe to run at scale
- avoid explosive queue growth
- remain incremental to implement
- preserve existing local development usability
- support future extension into more advanced scheduling and re-crawl strategies

## 4. Non-Goals

This spec does not include:

- full browser-rendered crawling for JS-heavy pages
- large-scale distributed crawl orchestration
- freshness recrawl scheduling beyond initial crawl admission
- full domain reputation modeling
- ML-based page quality scoring
- advanced canonical clustering
- anti-bot circumvention systems
- authentication-gated crawling

These may be future work, but they are outside this first expansion.

## 5. Definitions

### Seed
A URL intentionally introduced into the crawl frontier at startup or by an operator.

### Frontier
The queue of URLs eligible for future crawling, along with metadata such as score, depth, and scheduling state.

### Domain policy
The set of rules that determines whether a discovered URL is allowed to enter the crawl frontier.

### Robots enforcement
The process of consulting and applying `robots.txt` allow/disallow and `crawl-delay` rules.

### Controlled bypass
An explicit operator-configured exception that disables robots evaluation for selected domains while preserving crawler-defined politeness controls.

### Politeness
Crawler-side behavior that prevents excessive request frequency or concurrency against a domain.

## 6. Design Overview

The spider expansion is organized around five major layers:

1. **Seed ingestion**
2. **Domain policy**
3. **Robots enforcement**
4. **Controlled bypass**
5. **Frontier prioritization and scheduling**

The architecture remains queue-driven. The major change is that queue admission and fetch eligibility become much more deliberate.

At a high level, the crawler flow becomes:

1. ingest seeds
2. normalize and score them
3. enqueue seeds
4. workers pop frontier URLs
5. evaluate domain policy
6. apply robots or bypass rules
7. apply per-domain politeness
8. fetch content
9. extract links
10. normalize/filter discovered links
11. score and enqueue eligible links
12. persist crawl outputs

# 7. Seed Ingestion

## 7.1 Objective

Replace the current single-seed startup model with multi-seed ingestion.

## 7.2 Requirements

The spider must support:

- multiple seeds at startup
- seed validation
- seed normalization
- duplicate suppression
- seed priority assignment
- optional seed labeling for future campaign logic

## 7.3 Seed sources

### Phase 1: environment variable seeds

```env
STARTING_URLS=https://en.wikipedia.org/wiki/Web_crawler,https://news.ycombinator.com/,https://lobste.rs/
```

### Phase 2: structured config file

```json
[
  {
    "url": "https://news.ycombinator.com/",
    "priority": 100,
    "label": "tech-news"
  },
  {
    "url": "https://en.wikipedia.org/wiki/Web_crawler",
    "priority": 70,
    "label": "reference"
  }
]
```

### Phase 3: DB or Redis-backed seed campaigns

Examples:
- operator-defined crawl campaigns
- community-submitted seed batches
- time-bounded exploration sets

## 7.4 Seed schema

```go
type Seed struct {
    URL      string
    Priority float64
    Label    string
    Tags     []string
}
```

## 7.5 Seed ingestion rules

For each seed:

- trim whitespace
- validate scheme and host
- normalize the URL
- discard invalid seeds
- discard duplicate normalized seeds
- assign an initial frontier score derived from priority
- enqueue accepted seeds

## 7.6 Failure policy

- invalid seeds are logged and skipped
- duplicate seeds are ignored
- startup continues if at least one valid seed exists
- startup should fail clearly if no valid seeds are available

# 8. Domain Policy

## 8.1 Objective

Define where the crawler is allowed to go after discovering links.

## 8.2 Modes

### Mode A: same-domain
Only follow links within the originating seed domain or configured root domain set.

Use cases:
- single-site crawling
- debugging
- targeted crawling

### Mode B: allowlist
Only follow links whose domains are explicitly allowed.

Use cases:
- curated crawl universes
- vertical search
- trusted-source indexing

### Mode C: open-web with blocklist
Follow any eligible public URL except blocked domains.

Use cases:
- exploratory crawling
- discovery mode
- graph expansion beyond the seed universe

## 8.3 Domain policy schema

```go
type DomainPolicyConfig struct {
    Mode           string
    AllowedDomains []string
    BlockedDomains []string
}
```

## 8.4 Matching rules

The domain policy system must:

- normalize hostnames to lowercase
- strip port numbers
- support exact domain matching
- support optional subdomain matching
- reject private and loopback hosts
- reject unsupported schemes such as:
  - `mailto:`
  - `javascript:`
  - `tel:`
  - `data:`

## 8.5 Enforcement point

Domain policy must be enforced:

- before discovered URLs enter the frontier
- before fetch is attempted, as a defensive second check

## 8.6 Precedence

If multiple rules could apply:

- invalid scheme or host rejects the URL
- blocklist overrides allowlist
- domain policy is evaluated before robots enforcement
- a URL rejected by domain policy is never eligible for robots or politeness checks

# 9. Robots.txt, Politeness, and Controlled Bypass

## 9.1 Objective

Ensure the crawler behaves respectfully by default while supporting a tightly controlled exception path for explicitly approved domains.

The default posture is:

- respect `robots.txt`
- respect `crawl-delay`
- enforce per-domain politeness windows
- enforce per-domain concurrency caps

Controlled bypass is an exception path, not a replacement for default robots behavior.

## 9.2 Default robots enforcement flow

Before fetching a page, the spider must:

1. normalize the hostname
2. confirm domain-policy eligibility
3. check whether the domain is on the robots bypass list
4. if not bypassed:
   - fetch or reuse cached `robots.txt`
   - evaluate allow/disallow rules
   - read `crawl-delay` if present
5. apply politeness scheduling
6. fetch only if eligible

## 9.3 Robots cache

The spider should cache robots data per host.

```go
type RobotsCacheEntry struct {
    Host       string
    FetchedAt  time.Time
    ExpiresAt  time.Time
    CrawlDelay time.Duration
    Rules      any
}
```

Recommended TTL:
- 1 hour to 24 hours depending on implementation simplicity

## 9.4 Robots configuration

```go
type RobotsConfig struct {
    Enabled                bool
    CacheTTLSeconds        int
    DefaultCrawlDelayMs    int
    MaxConcurrentPerDomain int
    AllowOnFetchFailure    bool

    BypassDomains          []string
    BypassSubdomains       bool
    BypassDelayMs          int
}
```

### Meaning

- `Enabled`: turns robots enforcement on or off globally
- `CacheTTLSeconds`: cache life for fetched robots data
- `DefaultCrawlDelayMs`: fallback delay when no robots delay is given
- `MaxConcurrentPerDomain`: crawler-side concurrency cap
- `AllowOnFetchFailure`: policy for robots fetch failure
- `BypassDomains`: domains approved for controlled robots bypass
- `BypassSubdomains`: whether listed domains match subdomains
- `BypassDelayMs`: crawler-defined minimum delay for bypassed domains

## 9.5 Controlled robots bypass

### Purpose

Allow explicitly approved domains to bypass:

- robots allow/disallow evaluation
- robots crawl-delay directives

This is intended for domains that:

- are operationally important to the crawl strategy
- present public content but aggressively block generic crawlers
- have been deliberately reviewed and approved by an operator

### Non-goal

This is not a global “ignore robots” mode.

## 9.6 Bypass behavior

If a domain is on the bypass list:

- skip robots allow/disallow checks
- skip robots crawl-delay directives
- still apply crawler-defined politeness
- still apply:
  - blocklist rules
  - domain policy rules
  - per-domain concurrency caps
  - timeouts
  - retry/backoff logic
  - queue-pressure logic

Bypass means the crawler uses its own discipline instead of the site’s crawler instructions. It does not mean the crawler becomes unbounded.

## 9.7 Bypass matching rules

Bypass matching must:

- normalize hostnames
- strip ports
- support exact matches
- optionally support subdomain matches

Examples with `BypassSubdomains=true`:

- `reddit.com` matches `reddit.com`
- `reddit.com` matches `www.reddit.com`
- `reddit.com` matches `old.reddit.com`

Examples with `BypassSubdomains=false`:

- `reddit.com` matches only `reddit.com`

## 9.8 Politeness model

The crawler must maintain a domain-level schedule:

```go
type DomainSchedule struct {
    Host          string
    NextAllowedAt time.Time
    InFlightCount int
}
```

Politeness must enforce:

- minimum time between fetches to the same host
- maximum in-flight requests per host
- optional backoff when errors spike

This applies both to normal robots-enforced domains and bypassed domains.

## 9.9 Effective delay selection

The effective delay for a domain should be computed as follows:

### Non-bypassed domain
- if robots `crawl-delay` exists, use it
- otherwise use `DefaultCrawlDelayMs`

### Bypassed domain
- ignore robots delay
- use `BypassDelayMs`
- if `BypassDelayMs` is unset, fall back to `DefaultCrawlDelayMs`

## 9.10 Robots fetch failure policy

For non-bypassed domains:

- if robots fetch fails, apply `AllowOnFetchFailure`

Recommended first cut:
- allow temporarily, but log loudly

For bypassed domains:
- do not attempt robots enforcement
- proceed with crawler-defined politeness only

If a domain appears in both:
- bypass list
- blocklist

Then:
- **blocklist wins**

## 9.11 Observability

Bypassed fetches must be auditable.

Each bypassed fetch should log:

- matched host
- matched bypass rule
- whether it was an exact or subdomain match
- effective delay
- URL fetched

Example:

```text
event=robots_bypass_applied
host=www.reddit.com
matched_domain=reddit.com
subdomain_match=true
delay_ms=1500
url=https://www.reddit.com/r/programming/
```

Metrics should include:

- robots fetch successes/failures
- URLs denied by robots
- total bypassed fetches
- bypassed fetches by domain
- average delay by domain
- politeness deferrals by domain

## 9.12 Safety controls

Recommended guardrails:

```go
type BypassSafetyConfig struct {
    Enabled                 bool
    MaxPagesPerBypassedHost int
    MaxQueueSharePercent    int
}
```

Bypass safety should ensure:

- bypass applies only to explicit domains
- bypass does not override blocklists
- bypass does not disable politeness
- bypass remains visible in logs and metrics
- bypassed domains cannot monopolize the queue

## 9.13 Bypass governance contract

Bypass entries must be explicitly governed and time-bounded.

Each bypass entry should include:

- `domain`
- `requested_by`
- `approved_by`
- `justification`
- `created_at` (UTC)
- `expires_at` (UTC)
- `review_ticket`

Governance rules:

- default max approval window: 90 days
- expired entries must be treated as not bypassed
- recertification required before expiration
- blocklist always overrides bypass approvals
- all bypass changes must be auditable (ticket/linkable change record)

Operational recommendation:

- review active bypass list weekly
- include bypass count + upcoming expirations in weekly readiness review

# 10. Frontier Prioritization

## 10.1 Objective

Ensure the crawler prefers useful crawl paths rather than treating all discovered URLs equally.

## 10.2 High-level approach

Each frontier item should receive a score derived from:

- seed priority
- crawl depth
- domain preference
- link quality
- queue pressure
- politeness effects
- tie-break randomness

## 10.3 Score model

```text
final_score =
  seed_priority_component
+ depth_component
+ domain_preference_component
+ link_quality_component
- queue_pressure_penalty
- politeness_penalty
+ tie_breaker
```

## 10.4 Seed priority tiers

Example seed priority bands:

- `100` = high-trust or operator-curated
- `70` = normal curated seed
- `40` = exploratory seed
- `10` = low-confidence seed

These priorities influence initial queue position and downstream discovered-link inheritance.

## 10.5 Domain preference

Boost or penalize URLs based on domain properties:

Positive signals:
- allowlisted domain
- trusted domain family
- source domain continuity
- domain labeled as strategic

Negative signals:
- noisy or low-value domains
- domains already consuming too much crawl budget
- domains causing repeated errors or throttling

## 10.6 Link quality heuristics

Positive indicators:
- short clean paths
- article-like or content-like URLs
- low query-string complexity
- paths that appear content-bearing

Negative indicators:
- excessive query parameters
- tracking-heavy URLs
- login/account/session paths
- obvious sort/filter/tag explosion paths
- duplicate navigation patterns

## 10.7 Depth handling

Depth should not dominate the score completely, but should matter.

Suggested behavior:
- shallower links get preference
- deeper links remain eligible if otherwise strong
- exploration depth should taper rather than cliff-drop

## 10.8 Queue pressure and crawl budget

When the frontier is growing too quickly:

- penalize exploratory URLs
- penalize noisy domains
- preserve high-priority seeds
- preserve trusted domains
- reduce admission of low-value patterns

# 11. Combined Configuration

## 11.1 Unified spider config

```go
type SpiderConfig struct {
    Seeds []Seed

    DomainPolicy DomainPolicyConfig

    Robots RobotsConfig

    Frontier struct {
        MaxPages               int
        MaxConcurrency         int
        MaxQueueSize           int
        SameDomainBoost        float64
        AllowlistedDomainBoost float64
        QueryParamPenalty      float64
        DeepPathPenalty        float64
        SeedPriorityScale      float64
    }

    BypassSafety BypassSafetyConfig
}
```

## 11.2 Example environment mapping

```env
STARTING_URLS=https://news.ycombinator.com/,https://en.wikipedia.org/wiki/Web_crawler
CRAWL_MODE=allowlist
ALLOWED_DOMAINS=news.ycombinator.com,en.wikipedia.org,arstechnica.com,lobste.rs
BLOCKED_DOMAINS=facebook.com,instagram.com,localhost

ROBOTS_ENABLED=true
ROBOTS_CACHE_TTL_SECONDS=3600
DEFAULT_CRAWL_DELAY_MS=2000
MAX_CONCURRENT_PER_DOMAIN=1
ROBOTS_ALLOW_ON_FETCH_FAILURE=true

ROBOTS_BYPASS_DOMAINS=reddit.com,www.reddit.com,old.reddit.com
ROBOTS_BYPASS_SUBDOMAINS=true
ROBOTS_BYPASS_DELAY_MS=1500

BYPASS_MAX_PAGES_PER_HOST=5000
BYPASS_MAX_QUEUE_SHARE_PERCENT=20
```

# 12. Crawl Lifecycle

## 12.1 Startup flow

1. load configuration
2. parse seeds
3. normalize and validate seeds
4. deduplicate seeds
5. assign initial scores
6. enqueue accepted seeds

## 12.2 Worker loop

For each worker iteration:

1. pop next URL from frontier
2. normalize URL and host
3. re-check domain policy
4. determine robots bypass eligibility
5. if not bypassed, evaluate robots
6. compute effective politeness delay
7. check per-domain concurrency
8. fetch content
9. extract links and images
10. normalize and filter discovered links
11. score admitted URLs
12. enqueue admitted URLs
13. persist page data
14. mark page visited

## 12.3 Defensive checks

Even if URLs were filtered at enqueue time, fetch-time checks should repeat critical gates:

- host validity
- domain policy
- bypass eligibility
- politeness schedule

This prevents frontier corruption from becoming fetch corruption.

# 13. Data and State Additions

## 13.1 Existing state retained

The following existing Redis-backed concepts remain:

- spider queue
- visited URLs
- seen URLs

## 13.2 New state

Recommended new state:

- robots cache by host
- next allowed fetch time by host
- in-flight fetch count by host
- optional per-domain queue budgets
- optional bypass counters

## 13.3 Example Redis key shapes

```text
robots_cache:{host}
domain_next_fetch:{host}
domain_inflight:{host}
domain_budget:{host}
bypass_fetch_count:{host}
```

# 14. Failure Handling

## 14.1 Seed failures
- invalid seeds are skipped
- duplicate seeds are ignored
- startup continues if valid seeds remain

## 14.2 Robots failures
- if robots fetch fails for non-bypassed domains, use configured fallback
- if robots parsing fails, log and apply fallback
- bypassed domains do not depend on robots fetch outcome

## 14.3 Queue saturation
When frontier size approaches limits:

- reduce low-priority admissions
- penalize noisy discovered links
- preserve curated seeds and trusted domains

## 14.4 Domain throttling backlog
If many URLs are delayed by politeness windows:

- requeue or defer without hot-loop spinning
- avoid repeated immediate retries for the same host

## 14.5 Repeated failures on a domain
Recommended future behavior:
- apply temporary domain-specific backoff
- reduce crawl priority for failing hosts
- keep errors visible in metrics

# 15. Security and Operational Constraints

The spider must not:

- crawl unsupported schemes
- crawl localhost or private network targets
- let bypass override blocklists
- let any one host monopolize the crawl frontier
- disable politeness for convenience

The system should prefer auditable exceptions over silent behavior.

# 16. Implementation Phases

## Phase 1 — Multi-seed bootstrap and domain policy
Deliver:

- `STARTING_URLS`
- seed deduplication and normalization
- `CRAWL_MODE`
- `ALLOWED_DOMAINS`
- `BLOCKED_DOMAINS`
- discovered-link admission filtering

This gives the spider a wider opening gate.

## Phase 2 — Robots enforcement and politeness
Deliver:

- robots fetch and cache
- allow/disallow evaluation
- crawl-delay support
- per-domain scheduling
- per-domain concurrency limits

This gives the spider manners.

## Phase 3 — Controlled bypass
Deliver:

- explicit bypass domain list
- bypass subdomain matching
- crawler-defined bypass delay
- logging and metrics for bypass usage
- bypass safety caps

This gives the spider a disciplined exception path.

## Phase 4 — Frontier prioritization refinement
Deliver:

- seed priority tiers
- domain preference boosts
- junk URL penalties
- queue pressure penalties
- optional campaign labels

This gives the spider taste.

# 17. Acceptance Criteria

## 17.1 Multi-seed
- spider accepts multiple seeds at startup
- duplicate seeds are not re-enqueued
- invalid seeds are skipped safely
- seeds can be assigned different priority tiers

## 17.2 Domain policy
- same-domain mode never crosses configured domain boundaries
- allowlist mode only crawls configured domains
- open-web mode excludes blocked domains
- unsupported schemes are never admitted

## 17.3 Robots enforcement
- non-bypassed domains obey robots allow/disallow rules
- crawl-delay is respected when present
- fallback behavior is consistent when robots fetch fails

## 17.4 Controlled bypass
- domains on the bypass list are fetched without robots allow/disallow checks
- bypassed domains still obey crawler-defined delays and concurrency caps
- blocklisted domains are never crawled even if bypassed
- bypass usage is logged and measurable

## 17.5 Frontier prioritization
- higher-priority seeds are favored early
- trusted domains outrank exploratory noise
- junk URL patterns receive lower queue priority
- queue pressure reduces low-value admissions

# 18. Recommended First Cut for Moogle

For the current repo, the best first practical slice is:

- `STARTING_URLS`
- `CRAWL_MODE`
- `ALLOWED_DOMAINS`
- `BLOCKED_DOMAINS`
- simple seed priority
- robots cache
- default per-domain delay
- one in-flight request per domain
- controlled bypass list
- bypass logging
- light frontier penalties for junk URLs

That is enough to move the spider from a narrow seed demo into a real controlled crawler without detonating the codebase into a giant rewrite.

# 19. Suggested Initial Defaults

Use the canonical example in section **11.2 Example environment mapping**.

To avoid drift, keep only that single env snippet updated when defaults change.

# 20. Suggested Follow-On Engineering Tasks

1. update `main.go` to support `STARTING_URLS`
2. add seed parsing and normalization helpers
3. add a domain policy package
4. add robots cache and parser support
5. add per-domain schedule tracking
6. add bypass matching and audit logging
7. extend frontier score calculation
8. add tests for:
   - seed parsing
   - domain allow/block rules
   - robots allow/disallow
   - bypass logic
   - politeness delay enforcement
   - frontier priority behavior

# 21. Final Design Principle

The crawler should not become reckless just because it becomes capable.

Its growth should feel like a ship leaving a harbor:
first a careful widening of route,
then charts,
then weather sense,
then a captain’s discretion.

That is the heart of this spec:
more reach, more control, more intention.
