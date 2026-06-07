package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "requests_total", Help: "Total requests received"},
		[]string{"status"},
	)
	ActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{Name: "active_connections", Help: "Active WebSocket connections"},
	)
	RequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "request_latency_seconds", Help: "Request latency", Buckets: prometheus.DefBuckets},
		[]string{"source"},
	)
	CacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "cache_hits_total", Help: "Cache hits"},
		[]string{"layer"},
	)
	CacheMissesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Name: "cache_misses_total", Help: "Cache misses"},
	)
	TTFTSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "ttft_seconds", Help: "Time to first token", Buckets: prometheus.DefBuckets},
		[]string{"model"},
	)
	TokensPerSecond = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "tokens_per_second", Help: "Tokens generated per second", Buckets: prometheus.DefBuckets},
		[]string{"model"},
	)
	ProviderFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "provider_failures_total", Help: "Provider failures"},
		[]string{"provider"},
	)
)

func Register() {
	prometheus.MustRegister(
		RequestsTotal,
		ActiveConnections,
		RequestLatency,
		CacheHitsTotal,
		CacheMissesTotal,
		TTFTSeconds,
		TokensPerSecond,
		ProviderFailuresTotal,
	)
}
