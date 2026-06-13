package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	qdrantcache "github.com/harshagae/orchestration-gateway/internal/cache/qdrant"
	rediscache "github.com/harshagae/orchestration-gateway/internal/cache/redis"
	"github.com/harshagae/orchestration-gateway/internal/cache/embeddings"
	"github.com/harshagae/orchestration-gateway/internal/limiter"
	"github.com/harshagae/orchestration-gateway/internal/metrics"
	"github.com/harshagae/orchestration-gateway/internal/models"
	roundrobin "github.com/harshagae/orchestration-gateway/internal/router/round_robin"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Handler struct {
	redis    *rediscache.Cache
	qdrant   *qdrantcache.Cache
	embedder *embeddings.Service
	router   *roundrobin.Router
	limiter  *limiter.Limiter
}

func NewHandler(
	redis *rediscache.Cache,
	qdrant *qdrantcache.Cache,
	embedder *embeddings.Service,
	router *roundrobin.Router,
	lim *limiter.Limiter,
) *Handler {
	return &Handler{
		redis:    redis,
		qdrant:   qdrant,
		embedder: embedder,
		router:   router,
		limiter:  lim,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(r) {
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	h.limiter.Acquire(r)
	defer h.limiter.Release(r)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	metrics.ActiveConnections.Inc()
	defer metrics.ActiveConnections.Dec()
	slog.Info("websocket connection opened", "remote_addr", r.RemoteAddr)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req models.ChatRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			slog.Warn("invalid request payload", "raw", string(msg), "err", err)
			h.send(conn, models.ChatResponse{Error: "invalid request", Done: true})
			continue
		}

		slog.Info("prompt received", "message", req.Message, "remote_addr", r.RemoteAddr)
		h.handle(conn, r, req)
	}
}

func (h *Handler) handle(conn *websocket.Conn, r *http.Request, req models.ChatRequest) {
	start := time.Now()
	ctx := r.Context()

	slog.Info("[handler] request received", "message", req.Message, "remote_addr", r.RemoteAddr)
	metrics.RequestsTotal.WithLabelValues("received").Inc()

	// L1: Redis exact match
	slog.Info("[handler] checking L1 cache (redis)", "message", req.Message)
	if cached, err := h.redis.Get(ctx, req.Message); err == nil {
		slog.Info("[handler] L1 hit — returning cached response", "model", cached.Model, "latency_ms", time.Since(start).Milliseconds())
		metrics.CacheHitsTotal.WithLabelValues("redis").Inc()
		metrics.RequestLatency.WithLabelValues("redis").Observe(time.Since(start).Seconds())
		h.send(conn, models.ChatResponse{Token: cached.Response, Done: true, Source: "redis", Model: cached.Model})
		return
	}
	slog.Info("[handler] L1 miss — proceeding to L2 cache (qdrant)")

	// L2: Qdrant semantic match
	slog.Info("[handler] generating embedding for semantic search", "message", req.Message)
	embedding, err := h.embedder.Embed(ctx, req.Message)
	if err != nil {
		slog.Warn("[handler] embedding failed — skipping L2 cache", "err", err)
	} else {
		slog.Info("[handler] checking L2 cache (qdrant)")
		response, model, err := h.qdrant.Search(ctx, embedding)
		if err == nil && response != "" {
			slog.Info("[handler] L2 hit — returning cached response", "model", model, "latency_ms", time.Since(start).Milliseconds())
			metrics.CacheHitsTotal.WithLabelValues("qdrant").Inc()
			metrics.RequestLatency.WithLabelValues("qdrant").Observe(time.Since(start).Seconds())
			h.send(conn, models.ChatResponse{Token: response, Done: true, Source: "qdrant", Model: model})
			_ = h.redis.Set(ctx, req.Message, &models.CachedResponse{Response: response, Model: model})
			return
		}
		slog.Info("[handler] L2 miss — routing to provider")
	}

	// Cache miss — route to provider
	slog.Info("[handler] cache miss — selecting provider via router")
	metrics.CacheMissesTotal.Inc()

	provider, err := h.router.Next()
	if err != nil {
		slog.Error("[handler] no healthy provider available", "err", err)
		h.send(conn, models.ChatResponse{Error: "no healthy provider available", Done: true})
		return
	}
	slog.Info("[handler] streaming from provider", "provider", provider.Name())
	tokenCh := make(chan string, 64)

	var fullResponse strings.Builder
	ttftRecorded := false
	ttftStart := time.Now()

	go func() {
		if err := provider.Stream(ctx, req.Message, tokenCh); err != nil {
			slog.Error("[handler] provider stream error", "provider", provider.Name(), "err", err)
			metrics.ProviderFailuresTotal.WithLabelValues(provider.Name()).Inc()
		}
	}()

	for token := range tokenCh {
		if !ttftRecorded {
			ttft := time.Since(ttftStart).Seconds()
			slog.Info("[handler] first token received", "provider", provider.Name(), "ttft_ms", int(ttft*1000))
			metrics.TTFTSeconds.WithLabelValues(provider.Name()).Observe(ttft)
			ttftRecorded = true
		}
		fullResponse.WriteString(token)
		h.send(conn, models.ChatResponse{Token: token, Model: provider.Name()})
	}

	slog.Info("[handler] stream complete", "provider", provider.Name(), "total_latency_ms", time.Since(start).Milliseconds())
	h.send(conn, models.ChatResponse{Done: true, Source: "model", Model: provider.Name()})
	metrics.RequestLatency.WithLabelValues("model").Observe(time.Since(start).Seconds())

	// Populate caches asynchronously
	complete := fullResponse.String()
	slog.Info("[handler] populating caches async", "provider", provider.Name(), "response_length", len(complete))
	go func() {
		storeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cr := &models.CachedResponse{Response: complete, Model: provider.Name()}
		_ = h.redis.Set(storeCtx, req.Message, cr)
		if embedding != nil {
			_ = h.qdrant.Store(storeCtx, embedding, req.Message, complete, provider.Name())
		}
	}()
}

func (h *Handler) send(conn *websocket.Conn, resp models.ChatResponse) {
	b, _ := json.Marshal(resp)
	_ = conn.WriteMessage(websocket.TextMessage, b)
}
