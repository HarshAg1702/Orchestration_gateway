# Distributed Model Inference & Orchestration Gateway

## Project Overview

A high-throughput AI inference gateway responsible for:
- Receiving client requests over WebSockets
- Performing multi-level caching (Redis L1 + Qdrant L2)
- Routing requests across multiple LLM providers
- Streaming tokens back to clients
- Collecting operational metrics
- Supporting failover and future intelligent routing

**Primary goal:** Minimize inference costs while maximizing throughput and observability.

---

## Technology Stack

| Component | Technology |
|---|---|
| Language | Go |
| Communication | WebSocket |
| Models | Ollama |
| LLMs | Llama3, Mistral |
| Embeddings | nomic-embed-text |
| Exact Cache | Redis |
| Semantic Cache | Qdrant |
| Metrics | Prometheus |
| Visualization | Grafana |
| Containerization | Docker |
| Logging | Structured JSON Logs |
| Future Internal RPC | gRPC |

---

## Core Architecture

```
Client
  |
WebSocket
  |
Gateway
  |
Rate Limiter
  |
--------------------
|   Cache Layer    |
--------------------
  |
  +--> Redis (L1)     [Exact match, SHA256 key, TTL 24h, ~1-5ms]
  |
  +--> Qdrant (L2)    [Semantic match, cosine sim >= 0.95, TTL 7d, ~20-50ms]
  |
Cache Miss
  |
Router
  |
-------------------
|                 |
Llama3          Mistral
(Ollama)        (Ollama)
  |
Response
  |
Store Cache (Redis + Qdrant)
  |
Client
```

---

## Folder Structure

```
ai-gateway/
├── cmd/
│   └── gateway/
├── internal/
│   ├── api/
│   │   ├── websocket/
│   │   ├── handlers/
│   │   └── middleware/
│   ├── cache/
│   │   ├── redis/
│   │   ├── qdrant/
│   │   └── embeddings/
│   ├── providers/
│   │   ├── ollama/
│   │   └── interfaces/
│   ├── router/
│   │   ├── round_robin/
│   │   └── strategy/
│   ├── limiter/
│   ├── metrics/
│   ├── health/
│   ├── models/
│   ├── config/
│   └── logger/
├── deployments/
│   ├── docker/
│   ├── prometheus/
│   └── grafana/
├── configs/
└── docs/
```

---

## Request Lifecycle

### Step 1 — Client Connection
Client opens WebSocket connection:
```
ws://localhost:8080/chat
```

### Step 2 — Prompt Received
Gateway receives the prompt payload:
```json
{
  "message": "What is Kubernetes?"
}
```

### Step 3 — Rate Limiter Validation
Checks enforced:
- Request count
- Active connections
- IP limits

### Step 4 — Redis Cache Lookup (L1)
- Generate: `SHA256(prompt)`
- Lookup: `GET(prompt_hash)`
- **Hit:** Return cached response immediately
- **Miss:** Continue to L2

### Step 5 — Qdrant Semantic Cache Lookup (L2)
- Generate embedding using `nomic-embed-text`
- Search Qdrant: Top K = 3
- Condition: Similarity >= 0.95
- **Hit:** Return cached response
- **Miss:** Continue to router

### Step 6 — Router Selection
- **Current strategy:** Round Robin
- Future strategies: Latency Aware, Cost Aware, Load Aware

### Step 7 — Provider Execution
Selected model receives the request (Llama3 or Mistral via Ollama).

### Step 8 — Streaming Response
Tokens are streamed back over WebSocket incrementally:
```
H → He → Hel → Hell → Hello
```

### Step 9 — Cache Population
Store response in both caches:
- **Redis:** `{ prompt_hash → { response, model } }` with 24h TTL
- **Qdrant:** `{ embedding, prompt, response, metadata }` with 7d TTL

---

## Cache Layer Design

### L1 — Redis (Exact Match)

| Property | Value |
|---|---|
| Purpose | Exact match cache |
| Key | `SHA256(prompt)` |
| TTL | 24 hours |
| Expected Latency | 1–5 ms |

Stored value:
```json
{
  "response": "...",
  "model": "llama3"
}
```

### L2 — Qdrant (Semantic Cache)

| Property | Value |
|---|---|
| Purpose | Semantic similarity cache |
| Stores | Embeddings, prompt, response, metadata |
| Similarity | Cosine similarity |
| Threshold | 0.95 |
| TTL | 7 days |
| Expected Latency | 20–50 ms |

---

## Provider Abstraction

```go
type Provider interface {
    Stream(ctx context.Context, request Request) error
    Health() error
    Name() string
}
```

**Benefits:**
- Plug-and-play providers
- Future support: Gemini, vLLM, OpenAI

---

## Router Design

### Version 1 — Round Robin (Current)
```
Req1 → Llama3
Req2 → Mistral
Req3 → Llama3
```

### Version 2 — Latency Aware
Metrics used:
- Time to First Token (TTFT)
- Average latency
- Error rate

### Version 3 — Intelligent Routing
Factors:
- Prompt complexity
- Historical performance
- Queue length
- Provider health

---

## Metrics

### Gateway Metrics
- `requests_total`
- `active_connections`
- `request_latency`

### Cache Metrics
- `cache_hits_total`
- `cache_misses_total`
- `redis_hits_total`
- `qdrant_hits_total`
- `cache_hit_ratio`

### Model Metrics
- `ttft_seconds` — Time to First Token
- `tokens_per_second`
- `total_tokens_generated`
- `average_completion_time`

### Reliability Metrics
- `provider_failures_total`
- `provider_availability`
- `retry_count`

---

## Grafana Dashboards

### System Dashboard
- Active connections
- Requests per second
- Gateway latency

### Cache Dashboard
- Redis hit rate
- Qdrant hit rate
- Cost saved
- Latency saved

### Model Dashboard
- TTFT
- Throughput
- Error rate
- Provider availability

---

## Phase Roadmap

### Phase 1 — MVP
- [x] WebSocket Gateway
- [x] Ollama Integration (Llama3.2 + Mistral)
- [x] Streaming Responses
- [x] Prometheus Metrics

### Phase 2 — Caching
- [x] Redis Cache (L1)
- [x] Qdrant Semantic Cache (L2)
- [x] Embedding Service (`nomic-embed-text`)

### Phase 3 — Reliability
- [x] Rate Limiting
- [x] Health Checks
- [x] Failover Routing

### Phase 4 — Scale
- [x] Queueing Layer
- [x] Worker Pools
- [x] Backpressure Handling

### Phase 5 — Intelligence
- [ ] Intelligent Routing
- [ ] Dynamic Load Balancing
- [ ] Routing Analytics

---

## Project Goals

### Functional
- Stream LLM responses in real-time
- Support multiple providers
- Semantic caching
- Failover handling
- Health monitoring
- Request routing

### Non-Functional
- Low latency
- High throughput
- Extensible architecture
- Observability-first design
- Containerized deployment

---

## Success Criteria

- [x] Support multiple models (llama3.2 + mistral via Ollama)
- [x] Real-time token streaming over WebSocket
- [x] Redis L1 cache hit returning in ~2ms (vs ~60s model call)
- [x] Qdrant L2 semantic cache with 768-dim nomic-embed-text embeddings
- [x] Semantic cache hit ratio > 30% on repeat/similar queries
- [x] Provider failover — unhealthy providers skipped automatically
- [x] Rate limiting — global (1000), per-IP connections (50), per-IP rate (100 req/min)
- [x] Bounded job queue (100) + worker pool (3 workers) with backpressure
- [x] Thread-safe concurrent WebSocket writes via mutex-protected safeConn
- [x] Observable via Prometheus metrics + Grafana dashboards
- [x] Fully Dockerized local deployment (Docker Compose)
- [x] Extensible provider architecture via Provider interface
- [ ] Intelligent routing (latency-aware, cost-aware)
- [ ] Dynamic load balancing
- [ ] Routing analytics dashboard
