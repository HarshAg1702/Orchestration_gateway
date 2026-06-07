package limiter

import (
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
	if l.active.Load() >= l.maxConnections {
		return false
	}

	ip := r.RemoteAddr
	l.mu.Lock()
	if l.ipCount[ip] >= l.maxPerIP {
		l.mu.Unlock()
		return false
	}
	l.ipCount[ip]++
	l.mu.Unlock()

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
