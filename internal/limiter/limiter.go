package limiter

import (
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type ipState struct {
	activeConns  int
	requestTimes []time.Time // sliding window for request rate
}

type Limiter struct {
	maxConnections int64
	active         atomic.Int64

	mu           sync.Mutex
	ipState      map[string]*ipState
	maxPerIP     int
	maxRatePerIP int           // max requests per IP per window
	rateWindow   time.Duration // sliding window duration
}

func New(maxConnections, maxPerIP int) *Limiter {
	return &Limiter{
		maxConnections: int64(maxConnections),
		ipState:        make(map[string]*ipState),
		maxPerIP:       maxPerIP,
		maxRatePerIP:   100,           // 100 requests per IP per minute
		rateWindow:     time.Minute,
	}
}

func (l *Limiter) Allow(r *http.Request) bool {
	ip := r.RemoteAddr
	active := l.active.Load()

	slog.Info("[limiter] checking connection", "ip", ip, "active_connections", active, "max_connections", l.maxConnections)

	// Check 1: global active connection limit
	if active >= l.maxConnections {
		slog.Warn("[limiter] rejected — global connection limit reached", "ip", ip, "active", active, "max", l.maxConnections)
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.ipState[ip]
	if state == nil {
		state = &ipState{}
		l.ipState[ip] = state
	}

	// Check 2: per-IP active connection limit
	if state.activeConns >= l.maxPerIP {
		slog.Warn("[limiter] rejected — per-IP connection limit reached", "ip", ip, "ip_active", state.activeConns, "max_per_ip", l.maxPerIP)
		return false
	}

	// Check 3: per-IP request rate (sliding window)
	now := time.Now()
	cutoff := now.Add(-l.rateWindow)
	valid := state.requestTimes[:0]
	for _, t := range state.requestTimes {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	state.requestTimes = valid

	if len(state.requestTimes) >= l.maxRatePerIP {
		slog.Warn("[limiter] rejected — request rate limit reached", "ip", ip, "requests_in_window", len(state.requestTimes), "max_rate", l.maxRatePerIP, "window", l.rateWindow)
		return false
	}

	state.requestTimes = append(state.requestTimes, now)
	slog.Info("[limiter] connection allowed", "ip", ip, "ip_active_conns", state.activeConns, "requests_in_window", len(state.requestTimes))
	return true
}

func (l *Limiter) Acquire(r *http.Request) {
	l.active.Add(1)
	ip := r.RemoteAddr
	l.mu.Lock()
	if l.ipState[ip] != nil {
		l.ipState[ip].activeConns++
	}
	l.mu.Unlock()
	slog.Info("[limiter] connection acquired", "ip", ip, "total_active", l.active.Load())
}

func (l *Limiter) Release(r *http.Request) {
	l.active.Add(-1)
	ip := r.RemoteAddr
	l.mu.Lock()
	if s := l.ipState[ip]; s != nil && s.activeConns > 0 {
		s.activeConns--
	}
	l.mu.Unlock()
	slog.Info("[limiter] connection released", "ip", ip, "total_active", l.active.Load())
}
