# Content Recommendation Service

A backend service that provides personalized content recommendations based on user watch history, genre preferences, and content popularity. Built with Go (Fiber), PostgreSQL, and Redis.

---

## Table of Contents

1. [Setup Instructions](#1-setup-instructions)
2. [Architecture Overview](#2-architecture-overview)
3. [Design Decisions](#3-design-decisions)
4. [Performance Results](#4-performance-results)
5. [Trade-offs and Future Improvements](#5-trade-offs-and-future-improvements)

---

## 1. Setup Instructions

### Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Docker | 20.10+ | Container runtime |
| Docker Compose | v2+ | Service orchestration |
| Go | 1.23+ | Local development (optional) |
| k6 | latest | Performance testing (optional) |

### Project Structure

```
.
├── cmd/server/main.go              # Application entry point
├── internal/
│   ├── config/config.go            # Environment-based configuration
│   ├── models/models.go            # Data models / DTOs
│   ├── handler/handler.go          # HTTP handler layer (Fiber)
│   ├── service/service.go          # Business logic layer
│   ├── repository/repository.go    # Database access layer
│   ├── model/client.go             # Mock ML model client
│   └── cache/cache.go              # Redis cache layer
├── db/
│   ├── migrations/001_init.sql     # DDL: tables + indexes
│   └── seed/seed.sql               # Deterministic seed data
├── k6/                             # k6 load test scripts
├── Dockerfile                      # Multi-stage Go build
└── docker-compose.yml              # Full stack: app + postgres + redis
```

### Quick Start (One Command)

```bash
docker-compose up --build
```

This single command will:
1. Start **PostgreSQL 15** and auto-run migrations (`001_init.sql`) and seeding (`seed.sql`) via `docker-entrypoint-initdb.d`
2. Start **Redis 7** as the cache layer
3. Build and start the **Go application** on port `8080`

Health checks ensure the app only starts after Postgres and Redis are ready.

### Verify It's Running

```bash
curl http://localhost:8080/health
# {"status":"ok"}

curl http://localhost:8080/users/1/recommendations?limit=5
```

### Migrations and Seeding

Migrations and seed data run automatically when the Postgres container initializes for the first time. The SQL files are mounted into `/docker-entrypoint-initdb.d/`:

- `db/migrations/001_init.sql` — Creates all tables (`users`, `content`, `user_watch_history`, `restrict_country_genre`, `subscription_genre`) and indexes
- `db/seed/seed.sql` — Inserts deterministic seed data (fixed-seed random distribution)

To **reset the database** (drop existing data and re-seed):

```bash
docker-compose down -v   # removes the pgdata volume
docker-compose up --build
```

### Local Development (Without Docker)

```bash
# Start Postgres and Redis manually or via docker-compose for infra only
docker-compose up postgres redis -d

# Set environment variables
export DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres DB_NAME=recommendations DB_SSLMODE=disable
export REDIS_ADDR=localhost:6379

# Run the application
go run ./cmd/server
```

### Running k6 Tests

```bash
# Install k6: https://k6.io/docs/getting-started/installation/

# Single user load test (100 req/s for 1 minute)
k6 run k6/single_user_load.js

# Batch endpoint stress test (ramping VUs)
k6 run k6/batch_stress.js

# Cache effectiveness test (repeated requests)
k6 run k6/cache_effectiveness.js
```

---

## 2. Architecture Overview

### High-Level System Design

```
┌──────────┐     ┌──────────────────────────────────────────────────┐
│  Client   │────▶│                  Fiber HTTP Server                │
│ (k6/curl) │     │                    :8080                         │
└──────────┘     └──────┬───────────────────────────────────────────┘
                        │
                        ▼
               ┌────────────────┐
               │    Handler     │  Input validation, routing,
               │    Layer       │  error response formatting
               └───────┬────────┘
                       │
                       ▼
               ┌────────────────┐       ┌─────────────┐
               │    Service     │──────▶│  Redis Cache │
               │ (Business      │◀──────│  (TTL: 10m) │
               │  Logic) Layer  │       └─────────────┘
               └───────┬────────┘
                       │
              ┌────────┴─────────┐
              ▼                  ▼
     ┌────────────────┐  ┌──────────────┐
     │   Repository   │  │ Model Client │
     │   Layer        │  │ (Mock ML)    │
     └───────┬────────┘  └──────┬───────┘
             │                  │
             │       ┌──────────┘
             ▼       ▼
      ┌─────────────────┐
      │   PostgreSQL 15  │
      │  (via Repository)│
      └─────────────────┘
```

### Layer Descriptions

| Layer | Package | Responsibility |
|---|---|---|
| **Handler** | `internal/handler` | Parses HTTP requests, validates path/query params, maps errors to proper HTTP status codes (400/404/500/503), returns JSON responses |
| **Service** | `internal/service` | Orchestrates business logic: cache check → model inference → cache store. Manages batch concurrency with worker pools. Provides cache warming via cron |
| **Repository** | `internal/repository` | Pure database access. Provides both single-entity and batch queries. Avoids N+1 via `ANY($1)` array queries and in-memory join for batch unwatched content |
| **Model Client** | `internal/model` | Mocks an ML recommendation model. Fetches data via Repository, calculates genre preferences, applies scoring formula, simulates latency (30–50 ms) and random failures (1.5%) |
| **Cache** | `internal/cache` | Redis get/set with structured keys and JSON serialization. 10-minute TTL. Exposes cache hit/miss info in API responses |
| **Config** | `internal/config` | Reads environment variables with sensible defaults for all infra connections |

### Data Flow: Single User Recommendation

```
1. GET /users/1/recommendations?limit=10
2. Handler validates user_id (int64 > 0) and limit (int > 0)
3. Service checks Redis: key = "rec:user:1:limit:10"
4. Cache HIT  → return cached response with metadata.cache_hit = true
5. Cache MISS → Model Client:
   a. Repository.GetUserByID(1)           → user record
   b. Repository.GetUserWatchHistoryWithGenres(1)  → last 50 watched items with genres
   c. Repository.GetUnwatchedContent(1, 100)       → top 100 unwatched by popularity
   d. Repository.GetRestrictedGenresByCountry(user.country)
   e. Repository.GetAllowedGenresBySubscription(user.subscription_type)
   f. Sleep 30-50ms (simulated latency)
   g. 1.5% chance of returning ErrModelInference
   h. Score each candidate: popularity(40%) + genre_match(35%) + recency(15%) + noise(10%)
   i. Filter by country restrictions and subscription permissions
   j. Sort descending, take top N
6. Service stores result in Redis (TTL 10m)
7. Handler returns JSON with metadata.cache_hit = false
```

### Data Flow: Batch Recommendations

```
1. GET /recommendations/batch?page=1&limit=20
2. Handler validates page (≥1) and limit (1–100)
3. Service checks Redis: key = "rec:batch:page:1:limit:20"
4. Cache MISS → proceed:
   a. Repository.GetTotalUserCount()
   b. Repository.GetPaginatedUserIDs(page=1, limit=20)
   c. BULK prefetch (avoids N+1):
      - Repository.GetUsersByIDs([1..20])
      - Repository.GetBatchWatchHistoryWithGenres([1..20])
      - Repository.GetBatchUnwatchedContent([1..20], 100)
      - Restriction/permission data per unique country/subscription
   d. Spawn 5 concurrent goroutines (bounded worker pool)
   e. Each goroutine: Model.GenerateRecommendationsWithData(...)
   f. Collect results (success/failed), preserve order
   g. Build summary: success_count, failed_count, processing_time_ms
5. Cache store (TTL 10m)
6. Return batch response
```

### How the Model Integrates with Database Queries

The Model Client does **not** access the database directly. It receives data through the Repository layer:

- **Single user path**: Model Client calls `repo.GetUserByID`, `repo.GetUserWatchHistoryWithGenres`, `repo.GetUnwatchedContent`, etc. internally
- **Batch path**: Service pre-fetches all data in bulk and passes it to `model.GenerateRecommendationsWithData()` — the model receives pre-loaded structs and only computes scores

This separation means the model can be swapped out for a real ML service without changing the data access layer.

---

## 3. Design Decisions

### Caching Strategy

| Decision | Rationale |
|---|---|
| **Redis** as cache | In-memory, sub-millisecond reads, native TTL support, widely used in production |
| **10-minute TTL** | Balances freshness vs. performance — recommendations don't change drastically minute-to-minute, but a 10m window prevents stale data for active users |
| **Structured key design** | `rec:user:{id}:limit:{limit}` and `rec:batch:page:{page}:limit:{limit}` — includes limit/page in key so different query params hit different cache entries |
| **Cache warming via cron** | `robfig/cron` runs `WarmBatchCache` every 30 minutes (first run delayed 30 min from startup). Pre-populates batch pages so periodic analytics jobs hit warm cache |
| **Cache hit/miss in metadata** | Exposes `metadata.cache_hit` in every response for observability and debugging |

### Concurrency Control

| Decision | Rationale |
|---|---|
| **Bounded worker pool (5)** | Server spec says 2 cores, DB pool is 10 connections. 5 concurrent goroutines leaves headroom for other requests while still parallelizing per-user model inference |
| **Semaphore pattern** | Buffered channel `sem := make(chan struct{}, 5)` — simple, zero-dependency concurrency limiter |
| **Per-user failure isolation** | One user's model failure doesn't abort the batch. Failed users get `status: "failed"` entries in the response |

### Error Handling

| Status Code | When |
|---|---|
| `400 Bad Request` | Invalid `user_id`, `limit`, or `page` parameter |
| `404 Not Found` | User ID doesn't exist in the database |
| `500 Internal Server Error` | Unexpected database or system errors |
| `503 Service Unavailable` | Model inference failure (simulated 1.5% random failure) |

Philosophy: errors at the Handler layer are mapped to structured JSON `{"error": "...", "message": "..."}`. Service-layer errors propagate via Go's `error` interface with string prefixes (`model_unavailable:`) for Handler-level branching.

### Database Indexing Strategy

```sql
-- Users: fast lookup by country and subscription for filtering
CREATE INDEX idx_users_country ON users(country);
CREATE INDEX idx_users_subscription ON users(subscription_type);

-- Content: fast genre filtering and popularity-based sorting
CREATE INDEX idx_content_genre ON content(genre);
CREATE INDEX idx_content_popularity ON content(popularity_score DESC);

-- Watch history: the critical path
CREATE INDEX idx_watch_history_user ON user_watch_history(user_id);
CREATE INDEX idx_watch_history_content ON user_watch_history(content_id);
CREATE INDEX idx_watch_history_composite ON user_watch_history(user_id, watched_at DESC);
```

The **composite index** on `(user_id, watched_at DESC)` is the most important — it serves the `ORDER BY watched_at DESC LIMIT 50` query in a single index scan without a sort step.

### Scoring Algorithm Rationale

```
final_score = popularity(40%) + genre_match(35%) + recency(15%) + noise(10%)
```

| Component | Weight | Why |
|---|---|---|
| **Popularity** | 0.40 | Strong baseline — popular content is popular for a reason. Prevents cold-start issues for new users with no history |
| **Genre match** | 0.35 | Personalization driver — users who watch lots of action should see more action. Normalized genre counts (0.0–1.0) prevent any single genre from dominating |
| **Recency** | 0.15 | Time decay `1/(1 + days/365)` — newer content gets a slight boost. A 1-week-old item scores ~0.98, a 1-year-old item scores ~0.50 |
| **Noise** | 0.10 | `random.uniform(-0.05, 0.05) * 0.1` — controlled exploration. Prevents recommendations from becoming completely deterministic, enables serendipitous discovery |

---

## 4. Performance Results

### Expected k6 Test Results

> Note: Actual numbers depend on hardware. Below are representative results on a 2-core machine.

#### Single User Load Test (`k6/single_user_load.js`)

100 requests/second sustained for 1 minute:

| Metric | Expected Value |
|---|---|
| **Avg Latency** | ~40–80 ms (cache miss), ~2–5 ms (cache hit) |
| **P95 Latency** | < 500 ms |
| **P99 Latency** | < 1000 ms |
| **Throughput** | ~100 req/s sustained |
| **Error Rate** | < 2% (1.5% from simulated model failures) |

#### Batch Stress Test (`k6/batch_stress.js`)

Ramping from 1 to 50 VUs:

| Metric | Expected Value |
|---|---|
| **Avg Latency** | ~200–600 ms (depends on page size and cache state) |
| **P95 Latency** | < 2000 ms |
| **P99 Latency** | < 5000 ms |
| **Error Rate** | < 5% |

#### Cache Effectiveness Test (`k6/cache_effectiveness.js`)

Repeated requests to 5 users:

| Metric | Expected Value |
|---|---|
| **Cache Hit Rate** | > 90% after initial warm-up |
| **Avg Latency (cache hit)** | ~1–5 ms |
| **Avg Latency (cache miss)** | ~40–80 ms |

### Identified Bottlenecks

1. **Model inference latency**: The simulated 30–50 ms sleep per user is the dominant cost for cache-miss requests. In batch mode, the 5-worker pool means ~6 rounds of sleep for 30 users
2. **Database connection pool**: With `max_open_conns=10`, concurrent batch processing can saturate connections during the bulk prefetch phase
3. **Batch page size**: Large `limit` values (e.g., 100) increase both DB query time and total model inference time linearly
4. **Redis serialization**: JSON marshal/unmarshal for large batch results adds a few milliseconds

### Cache Hit Rate Analysis

- **First request** for any user+limit combination: always a cache miss
- **Subsequent requests** within 10 minutes: cache hit (< 5 ms response)
- With 30 users and the cache effectiveness test hitting only 5 users repeatedly, hit rate quickly converges to > 95%
- The cron-based cache warming ensures batch endpoints are warm for periodic consumers

---

## 5. Trade-offs and Future Improvements

### Known Limitations

| Limitation | Impact |
|---|---|
| **No cache invalidation on watch history update** | If a user watches new content, cached recommendations remain stale until TTL expires (10 min). The prompt mentions this as a concern but no write endpoint exists yet |
| **In-memory union for batch unwatched content** | `GetBatchUnwatchedContent` fetches all content + all watch records for the user set and joins in Go. Works for 60 content items but won't scale to millions |
| **Single Redis instance** | No replication or clustering. Redis failure means all requests become cache misses |
| **Simulated model is synchronous** | Real ML models would likely be called over gRPC/HTTP with proper circuit breakers and timeouts |
| **No authentication/authorization** | Endpoints are fully open |
| **Fixed concurrency limit** | The 5-worker pool is hardcoded. Should be configurable based on deployment environment |
| **Per-call semaphore, not global** | `sem` is a local variable created fresh on every `GetBatchRecommendations` call. Two simultaneous batch requests on the same pod each get their own independent semaphore, so actual concurrent goroutines can reach `maxConcurrency × concurrent_requests` per pod, not `maxConcurrency` globally |
| **No cross-pod concurrency control** | In a Kubernetes deployment with multiple replicas, each pod maintains its own semaphore with no coordination. With 3 pods and `maxConcurrency=5`, up to 15 model/DB goroutines can run simultaneously across the cluster. If pods share host CPU without defined resource limits (`resources.limits.cpu`), this causes CPU contention and unpredictable latency spikes across all pods on the same node |

### Scalability Considerations

- **Horizontal scaling**: The app is stateless (cache in Redis, data in Postgres). Multiple instances can run behind a load balancer. However, because concurrency control is local per-pod (see Known Limitations above), horizontal scaling multiplies total goroutine concurrency linearly — define `resources.requests.cpu` and `resources.limits.cpu` in the pod spec to prevent CPU starvation between pods on the same node. For true cluster-wide concurrency control, a distributed semaphore
- **Database**: At scale, the `NOT IN (SELECT ...)` subquery for unwatched content should be replaced with a materialized view or a pre-computed "candidates" table
- **Redis Cluster**: For high availability, deploy Redis in cluster mode with read replicas
- **Connection pooling**: Consider pgbouncer in front of Postgres for connection multiplexing at higher request volumes
- **Content growth**: With millions of content items, the scoring algorithm should be moved to a separate service with pre-filtered candidate sets (e.g., collaborative filtering pre-pass)

### Proposed Enhancements

1. **Cache invalidation endpoint**: `POST /users/{id}/watch` that clears the user's recommendation cache key when new watch history is recorded
2. **Circuit breaker for model client**: Use a library like `sony/gobreaker` to prevent cascading failures when model inference repeatedly fails
3. **Structured logging**: Replace `log.Printf` with structured logging (e.g., `zerolog` or `zap`) for better observability in production
4. **Graceful shutdown**: Handle `SIGTERM`/`SIGINT` to drain in-flight requests and close DB/Redis connections cleanly
5. **Metrics endpoint**: Expose Prometheus metrics (`/metrics`) for request latency, cache hit rate, DB query duration, and worker pool utilization
6. **Rate limiting**: Add per-IP or per-user rate limiting to prevent abuse of the batch endpoint
7. **Real ML integration**: Replace the mock model with a gRPC client calling a TensorFlow Serving or similar inference endpoint
8. **Database read replicas**: Route read-heavy recommendation queries to a Postgres read replica
9. **Content pre-filtering**: Add a materialized view or async job that pre-computes candidate content per user segment (country + subscription) to reduce query-time filtering
10. **A/B testing support**: Add a `model_version` field to recommendations and cache keys to support running multiple scoring algorithms simultaneously
