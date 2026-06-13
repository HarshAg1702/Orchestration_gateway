package roundrobin

import (
	"log/slog"
	"sync/atomic"

	"github.com/harshagae/orchestration-gateway/internal/providers/interfaces"
)

type Router struct {
	providers []interfaces.Provider
	counter   atomic.Uint64
}

func New(providers []interfaces.Provider) *Router {
	return &Router{providers: providers}
}

func (r *Router) Next() interfaces.Provider {
	n := r.counter.Add(1)
	selected := r.providers[(n-1)%uint64(len(r.providers))]
	slog.Info("[router] provider selected", "provider", selected.Name(), "request_count", n)
	return selected
}
