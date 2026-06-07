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

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req models.ChatRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			h.send(conn, models.ChatResponse{Error: "invalid request", Done: true})
			continue
		}

		h.handle(conn, r, req)
	}
}

func (h *Handler) handle(conn *websocket.Conn, r *http.Request, req models.ChatRequest) {
	start := time.Now()
	ctx := r.Context()

	metrics.RequestsTotal.WithLabelValues("received").Inc()

	// L1: Redis exact match
	if cached, err := h.redis.Get(ctx, req.Message); err == nil {
		metrics.CacheHitsTotal.WithLabelValues("redis").Inc()
		metrics.RequestLatency.WithLabelValues("redis").Observe(time.Since(start).Seconds())
		h.send(conn, models.ChatResponse{Token: cached.Response, Done: true, Source: "redis", Model: cached.Model})
		return
	}

	// L2: Qdrant semantic match
	embedding, err := h.embedder.Embed(ctx, req.Message)
	if err != nil {
		slog.Warn("embed failed", "err", err)
	} else {
		response, model, err := h.qdrant.Search(ctx, embedding)
		if err == nil && response != "" {
			metrics.CacheHitsTotal.WithLabelValues("qdrant").Inc()
			metrics.RequestLatency.WithLabelValues("qdrant").Observe(time.Since(start).Seconds())
			h.send(conn, models.ChatResponse{Token: response, Done: true, Source: "qdrant", Model: model})
			_ = h.redis.Set(ctx, req.Message, &models.CachedResponse{Response: response, Model: model})
			return
		}
	}

	// Cache miss — route to provider
	metrics.CacheMissesTotal.Inc()

	provider := h.router.Next()
	tokenCh := make(chan string, 64)

	var fullResponse strings.Builder
	ttftRecorded := false
	ttftStart := time.Now()

	go func() {
		if err := provider.Stream(ctx, req.Message, tokenCh); err != nil {
			slog.Error("provider stream error", "provider", provider.Name(), "err", err)
			metrics.ProviderFailuresTotal.WithLabelValues(provider.Name()).Inc()
		}
	}()

	for token := range tokenCh {
		if !ttftRecorded {
			metrics.TTFTSeconds.WithLabelValues(provider.Name()).Observe(time.Since(ttftStart).Seconds())
			ttftRecorded = true
		}
		fullResponse.WriteString(token)
		h.send(conn, models.ChatResponse{Token: token, Model: provider.Name()})
	}

	h.send(conn, models.ChatResponse{Done: true, Source: "model", Model: provider.Name()})
	metrics.RequestLatency.WithLabelValues("model").Observe(time.Since(start).Seconds())

	// Populate caches asynchronously
	complete := fullResponse.String()
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
