package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type MetricType int

const (
	CounterMetricType MetricType = iota
	GaugeMetricType
	SummaryMetricType
	HistogramMetricType
)

type NameHash uint64

type ValueHash uint64

type LabelHash struct {
	// This is a hash over the label names
	Names NameHash
	// This is a hash over the label names + label values
	Values ValueHash
}

type MetricHolder interface{}

type VectorHolder interface {
	Delete(label prometheus.Labels) bool
}

type Vector struct {
	Holder   VectorHolder
	RefCount uint64
}

type Metric struct {
	MetricType MetricType
	// Vectors key is the hash of the label names
	Vectors map[NameHash]*Vector
	// Metrics key is a hash of the label names + label values
	Metrics map[ValueHash]*RegisteredMetric
}

type RegisteredMetric struct {
	LastRegisteredAt time.Time
	Labels           prometheus.Labels
	TTL              time.Duration
	Metric           MetricHolder
	VecKey           NameHash
}
