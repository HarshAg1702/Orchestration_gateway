package roundrobin

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/harshagae/orchestration-gateway/internal/providers/interfaces"
)

type Router struct {
	providers []interfaces.Provider
	counter   atomic.Uint64
}

func New(providers []interfaces.Provider) *Router {
	return &Router{providers: providers}
}

// Next returns the next healthy provider in round-robin order.
// It tries each provider once. If all are unhealthy it returns an error.
func (r *Router) Next() (interfaces.Provider, error) {
	total := uint64(len(r.providers))

	for range r.providers {
		n := r.counter.Add(1)
		candidate := r.providers[(n-1)%total]

		slog.Info("[router] checking provider health", "provider", candidate.Name())

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := candidate.Health(ctx)
		cancel()

		if err != nil {
			slog.Warn("[router] provider unhealthy — skipping", "provider", candidate.Name(), "err", err)
			continue
		}

		slog.Info("[router] provider selected", "provider", candidate.Name(), "request_count", n)
		return candidate, nil
	}

	slog.Error("[router] all providers unhealthy — no provider available")
	return nil, fmt.Errorf("all providers unhealthy")
}
