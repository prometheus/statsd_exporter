package mapper_cache

import "github.com/prometheus/client_golang/prometheus"

type CacheMetrics struct {
	CacheLength    prometheus.Gauge
	CacheGetsTotal prometheus.Counter
	CacheHitsTotal prometheus.Counter
}

func NewCacheMetrics(reg prometheus.Registerer) *CacheMetrics {
	var m CacheMetrics

	m.CacheLength = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "statsd_metric_mapper_cache_length",
			Help: "The count of unique metrics currently cached.",
		},
	)
	m.CacheGetsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_metric_mapper_cache_gets_total",
			Help: "The count of total metric cache gets.",
		},
	)
	m.CacheHitsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_metric_mapper_cache_hits_total",
			Help: "The count of total metric cache hits.",
		},
	)

	if reg != nil {
		reg.MustRegister(m.CacheLength)
		reg.MustRegister(m.CacheGetsTotal)
		reg.MustRegister(m.CacheHitsTotal)
	}
	return &m
}
