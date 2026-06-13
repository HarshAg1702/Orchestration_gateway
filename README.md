# Distributed Model Inference & Orchestration Gateway

A high-throughput AI inference gateway built in Go that routes requests across multiple LLM providers, caches responses at two levels, enforces rate limits, and streams tokens back to clients over WebSockets.

---

## Why We Built This

Running LLM inference is expensive and slow. Every request to a model costs time (30–60 seconds on CPU) and compute. This gateway solves three problems:

1. **Cost** — Cache responses so the same (or similar) question never hits the model twice
2. **Throughput** — Queue and pool requests so the model isn't overwhelmed
3. **Reliability** — Health-check providers and failover automatically

---

## Architecture

```
Client
  │
  │  WebSocket (ws://localhost:8080/chat)
  ▼
Gateway
  │
  ├── Rate Limiter (global + per-IP connections + per-IP request rate)
  │
  ├── L1 Cache: Redis (exact match, SHA256 key, 24h TTL, ~1-5ms)
  │
  ├── L2 Cache: Qdrant (semantic similarity, cosine >= 0.95, 7d TTL, ~20-50ms)
  │       └── Embeddings via nomic-embed-text (768 dimensions)
  │
  ├── Queue (bounded, max 100 jobs)
  │
  ├── Worker Pool (3 workers)
  │
  └── Router (Round Robin with health-aware failover)
          ├── llama3.2 (Ollama)
          └── mistral  (Ollama)
```

---

## Request Lifecycle

```
1. Client connects over WebSocket
2. Rate limiter validates (global limit, per-IP connections, per-IP request rate)
3. Prompt arrives as JSON: {"message": "What is Kubernetes?"}
4. Redis lookup (SHA256 hash of prompt)
      HIT  → return cached response immediately (~2ms)
      MISS → continue
5. Generate embedding via nomic-embed-text
6. Qdrant semantic search (cosine similarity >= 0.95)
      HIT  → return cached response (~150ms) + backfill Redis
      MISS → continue
7. Enqueue job → worker picks it up
8. Router selects healthy provider (round robin with health check)
9. Provider streams tokens back over WebSocket
10. Full response stored async in Redis + Qdrant
```

---

## Technology Stack

| Component | Technology | Purpose |
|---|---|---|
| Language | Go | High-performance, low-overhead |
| Transport | WebSocket (gorilla/websocket) | Real-time token streaming |
| Models | Ollama | Local LLM inference |
| LLMs | llama3.2, mistral | Text generation |
| Embeddings | nomic-embed-text | Semantic similarity vectors |
| L1 Cache | Redis | Exact match, low latency |
| L2 Cache | Qdrant | Semantic similarity search |
| Metrics | Prometheus | Operational observability |
| Dashboards | Grafana | Visualization |
| Containers | Docker Compose | Local deployment |
| Logging | structured JSON (log/slog) | Machine-readable logs |

---

## Optimizations

### Two-Level Caching
- **L1 Redis** handles exact repeat questions instantly (2ms vs 60s)
- **L2 Qdrant** handles semantically similar questions — "What is Kubernetes?" and "Explain Kubernetes to me" can share a cached response
- Cache population is **async** — response is streamed to client immediately, caches are written in the background

### Worker Pool + Queue
- **Problem:** 100 concurrent requests all hitting Ollama at once → model chokes
- **Solution:** Bounded queue (max 100 jobs) + fixed worker pool (3 workers) → controlled concurrency
- **Backpressure:** Queue full → client gets `{"error": "server busy, try again later"}` immediately instead of hanging

### Health-Aware Routing
- Router calls `Health()` on each provider before selecting it
- Unhealthy provider is skipped, next one is tried
- All providers down → clear error returned to client
- Round robin ensures load is spread evenly across healthy providers

### Thread-Safe WebSocket Writes
- gorilla/websocket does not allow concurrent writes
- All writes go through a mutex-protected `safeConn` wrapper
- Worker goroutine can stream tokens while the main goroutine reads new messages safely

### Rate Limiting (3 layers)
1. Global active connection limit (1000)
2. Per-IP active connection limit (50)
3. Per-IP request rate (100 requests/minute, sliding window)

---

## Folder Structure

```
orchestration_gateway/
├── cmd/
│   └── gateway/
│       └── main.go              # Entry point, wires everything together
├── internal/
│   ├── api/
│   │   └── websocket/
│   │       └── handler.go       # WebSocket handler, request lifecycle
│   ├── cache/
│   │   ├── redis/               # L1 exact match cache
│   │   ├── qdrant/              # L2 semantic cache
│   │   └── embeddings/          # nomic-embed-text via Ollama
│   ├── providers/
│   │   ├── ollama/              # Ollama streaming provider
│   │   └── interfaces/          # Provider interface (Stream, Health, Name)
│   ├── router/
│   │   └── round_robin/         # Health-aware round robin router
│   ├── queue/                   # Bounded job queue
│   ├── worker/                  # Fixed-size worker pool
│   ├── limiter/                 # Rate limiter (global + per-IP + rate window)
│   ├── metrics/                 # Prometheus metrics registration
│   ├── health/                  # /health endpoint
│   ├── models/                  # Shared types (ChatRequest, ChatResponse)
│   ├── config/                  # Env-based config loading
│   └── logger/                  # Structured JSON logger init
├── deployments/
│   ├── docker/
│   │   ├── Dockerfile           # Multi-stage Go build
│   │   └── docker-compose.yml   # Full stack: gateway + redis + qdrant + ollama + prometheus + grafana
│   └── prometheus/
│       └── prometheus.yml       # Scrape config
├── configs/
│   └── config.env               # Default configuration
└── README.md
```

---

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (running)
- [wscat](https://github.com/websockets/wscat) for testing (`npm install -g wscat`)

---

## Running the Project

### Step 1 — Start all services

```bash
docker compose -f deployments/docker/docker-compose.yml up --build
```

This starts: gateway, Redis, Qdrant, Ollama, Prometheus, Grafana.

### Step 2 — Pull models (first time only)

In a second terminal:

```bash
docker exec -it docker-ollama-1 ollama pull nomic-embed-text
docker exec -it docker-ollama-1 ollama pull llama3.2
docker exec -it docker-ollama-1 ollama pull mistral
```

> `nomic-embed-text` is ~274MB. `llama3.2` is ~2GB. `mistral` is ~4GB.

### Step 3 — Connect and send messages

```bash
wscat -c ws://localhost:8080/chat
```

```json
{"message": "What is Kubernetes?"}
```

---

## Testing the Cache

```bash
# Flush Redis to start clean
docker exec -it docker-redis-1 redis-cli FLUSHALL

wscat -c ws://localhost:8080/chat
```

| Test | Expected behaviour |
|---|---|
| `{"message": "What is Kubernetes?"}` | Cold request — hits model, stores in Redis + Qdrant |
| `{"message": "What is Kubernetes?"}` | Redis L1 hit — returns in ~2ms |
| `{"message": "Explain Kubernetes to me"}` | Qdrant L2 semantic hit (if similarity >= 0.95) |
| `{"message": "What is Docker?"}` | Cache miss — routes to next provider |

---

## Service URLs

| Service | URL |
|---|---|
| Gateway WebSocket | `ws://localhost:8080/chat` |
| Gateway Health | `http://localhost:8080/health` |
| Prometheus Metrics | `http://localhost:8080/metrics` |
| Prometheus UI | `http://localhost:9090` |
| Grafana | `http://localhost:3000` (admin / admin) |
| Qdrant Dashboard | `http://localhost:6333/dashboard` |

---

## Configuration

All config is in `configs/config.env`. The Docker Compose file overrides the service addresses for container networking.

| Variable | Default | Description |
|---|---|---|
| `SERVER_PORT` | `8080` | Gateway listen port |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama base URL |
| `LLAMA_MODEL` | `llama3.2` | Primary model name |
| `MISTRAL_MODEL` | `llama3.2` | Secondary model name |
| `EMBED_MODEL` | `nomic-embed-text` | Embedding model |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_TTL_HOURS` | `24` | Redis cache TTL |
| `QDRANT_ADDR` | `localhost:6333` | Qdrant REST address |
| `QDRANT_COLLECTION` | `prompt_cache` | Qdrant collection name |
| `QDRANT_THRESHOLD` | `0.95` | Cosine similarity threshold |
| `QDRANT_TTL_DAYS` | `7` | Qdrant cache TTL |

---

## Metrics

| Metric | Description |
|---|---|
| `requests_total` | Total requests by status |
| `active_connections` | Current active WebSocket connections |
| `request_latency` | End-to-end latency by source (redis/qdrant/model) |
| `cache_hits_total` | Cache hits by layer (redis/qdrant) |
| `cache_misses_total` | Total cache misses |
| `ttft_seconds` | Time to first token per provider |
| `provider_failures_total` | Provider errors per model |

---

## Useful Commands

```bash
# Start everything
docker compose -f deployments/docker/docker-compose.yml up --build

# Start in background
docker compose -f deployments/docker/docker-compose.yml up -d

# View gateway logs only
docker compose -f deployments/docker/docker-compose.yml logs -f gateway

# Stop everything
docker compose -f deployments/docker/docker-compose.yml down

# Stop and remove volumes (wipes Redis + Qdrant data)
docker compose -f deployments/docker/docker-compose.yml down -v

# Flush Redis cache
docker exec -it docker-redis-1 redis-cli FLUSHALL

# List loaded Ollama models
docker exec -it docker-ollama-1 ollama list

# Pull a model
docker exec -it docker-ollama-1 ollama pull llama3.2
```

---

## Roadmap

| Phase | Feature | Status |
|---|---|---|
| 1 | WebSocket Gateway, Ollama streaming, Prometheus metrics | ✅ Done |
| 2 | Redis L1 cache, Qdrant L2 semantic cache, embeddings | ✅ Done |
| 3 | Rate limiting, health checks, failover routing | ✅ Done |
| 4 | Job queue, worker pool, backpressure | ✅ Done |
| 5 | Latency-aware routing, intelligent routing, load balancing | 🔜 Planned |
