package limiter

import (
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
)

type Limiter struct {
	maxConnections int64
	active         atomic.Int64

	mu      sync.Mutex
	ipCount map[string]int
	maxPerIP int
}

func New(maxConnections, maxPerIP int) *Limiter {
	return &Limiter{
		maxConnections: int64(maxConnections),
		ipCount:        make(map[string]int),
		maxPerIP:       maxPerIP,
	}
}

func (l *Limiter) Allow(r *http.Request) bool {
	ip := r.RemoteAddr
	active := l.active.Load()

	slog.Info("[limiter] checking connection", "ip", ip, "active_connections", active, "max_connections", l.maxConnections)

	if active >= l.maxConnections {
		slog.Warn("[limiter] rejected — global limit reached", "ip", ip, "active", active, "max", l.maxConnections)
		return false
	}

	l.mu.Lock()
	ipActive := l.ipCount[ip]
	if ipActive >= l.maxPerIP {
		l.mu.Unlock()
		slog.Warn("[limiter] rejected — per-IP limit reached", "ip", ip, "ip_active", ipActive, "max_per_ip", l.maxPerIP)
		return false
	}
	l.ipCount[ip]++
	l.mu.Unlock()

	slog.Info("[limiter] connection allowed", "ip", ip)
	return true
}

func (l *Limiter) Acquire(r *http.Request) {
	l.active.Add(1)
}

func (l *Limiter) Release(r *http.Request) {
	l.active.Add(-1)
	ip := r.RemoteAddr
	l.mu.Lock()
	if l.ipCount[ip] > 0 {
		l.ipCount[ip]--
	}
	l.mu.Unlock()
}
