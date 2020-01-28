// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expiringregistry

import (
	"bytes"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

// uncheckedCollector wraps a Collector but its Describe method yields no Desc.
// This allows incoming metrics to have inconsistent label sets
type uncheckedCollector struct {
	c prometheus.Collector
}

func (u uncheckedCollector) Describe(_ chan<- *prometheus.Desc) {}
func (u uncheckedCollector) Collect(c chan<- prometheus.Metric) {
	u.c.Collect(c)
}

type metricType int

// metricType enums
const (
	CounterMetricType metricType = iota
	GaugeMetricType
	SummaryMetricType
	HistogramMetricType
)

type nameHash uint64
type valueHash uint64
type labelHash struct {
	// This is a hash over the label names
	names nameHash
	// This is a hash over the label names + label values
	values valueHash
}

type metricHolder interface{}

type registeredMetric struct {
	lastRegisteredAt time.Time
	labels           prometheus.Labels
	ttl              time.Duration
	metric           metricHolder
	vecKey           nameHash
}

type vectorHolder interface {
	Delete(label prometheus.Labels) bool
}

type vector struct {
	holder   vectorHolder
	refCount uint64
}

type metric struct {
	metricType metricType
	// Vectors key is the hash of the label names
	vectors map[nameHash]*vector
	// Metrics key is a hash of the label names + label values
	metrics map[valueHash]*registeredMetric
}

// Registry is an expiring metric registry
type Registry struct {
	mtx     sync.RWMutex
	metrics map[string]metric
	// The below value and label variables are allocated in the registry struct
	// so that we don't have to allocate them every time have to compute a label
	// hash.
	defaults          *mapper.MapperConfigDefaults
	metricsCount      *prometheus.GaugeVec // the prometheus gaugevec to add metric counts to
	valueBuf, nameBuf bytes.Buffer
	hasher            hash.Hash64
}

// NewRegistry returns a new expiring registry. Pass nil for metricsCount to use the default metric name for counts
func NewRegistry(defaults *mapper.MapperConfigDefaults, metricsCount *prometheus.GaugeVec) *Registry {
	if metricsCount == nil {
		metricsCount = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "expiring_registry_metrics_total",
				Help: "The total number of metrics.",
			},
			[]string{"type"})
	}
	return &Registry{
		metrics:      make(map[string]metric),
		metricsCount: metricsCount,
		defaults:     defaults,
		hasher:       fnv.New64a(),
	}
}

func (r *Registry) metricConflicts(metricName string, metricType metricType) bool {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	vector, hasMetric := r.metrics[metricName]
	if !hasMetric {
		// No metric with this name exists
		return false
	}

	if vector.metricType == metricType {
		// We've found a copy of this metric with this type, but different
		// labels, so it's safe to create a new one.
		return false
	}

	// The metric exists, but it's of a different type than we're trying to
	// create.
	return true
}

// storeCounter stores a counter with a ttl
func (r *Registry) storeCounter(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.CounterVec, c prometheus.Counter, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, c, CounterMetricType, ttl)
}

// storeGauge stores a gauge with a ttl
func (r *Registry) storeGauge(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.GaugeVec, g prometheus.Counter, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, g, GaugeMetricType, ttl)
}

// storeHistogram stores a histogram with a ttl
func (r *Registry) storeHistogram(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.HistogramVec, o prometheus.Observer, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, o, HistogramMetricType, ttl)
}

// storeSummary stores a summary with a ttl
func (r *Registry) storeSummary(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.SummaryVec, o prometheus.Observer, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, o, SummaryMetricType, ttl)
}

func (r *Registry) store(metricName string, hash labelHash, labels prometheus.Labels, vh vectorHolder, mh metricHolder, metricType metricType, ttl time.Duration) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	metric, hasMetric := r.metrics[metricName]
	if !hasMetric {
		metric.metricType = metricType
		metric.vectors = make(map[nameHash]*vector)
		metric.metrics = make(map[valueHash]*registeredMetric)

		r.metrics[metricName] = metric
	}

	v, ok := metric.vectors[hash.names]
	if !ok {
		v = &vector{holder: vh}
		metric.vectors[hash.names] = v
	}

	rm, ok := metric.metrics[hash.values]
	if !ok {
		rm = &registeredMetric{
			labels: labels,
			ttl:    ttl,
			metric: mh,
			vecKey: hash.names,
		}
		metric.metrics[hash.values] = rm
		v.refCount++
	}
	now := clock.Now()
	rm.lastRegisteredAt = now
	// Update ttl from mapping
	rm.ttl = ttl
}

func (r *Registry) get(metricName string, hash labelHash, metricType metricType) (vectorHolder, metricHolder) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	metric, hasMetric := r.metrics[metricName]

	if !hasMetric {
		return nil, nil
	}
	if metric.metricType != metricType {
		return nil, nil
	}

	rm, ok := metric.metrics[hash.values]
	if ok {
		now := clock.Now()
		rm.lastRegisteredAt = now
		return metric.vectors[hash.names].holder, rm.metric
	}

	vector, ok := metric.vectors[hash.names]
	if ok {
		return vector.holder, nil
	}

	return nil, nil
}

// GetCounter gets a prometheus.Counter from the ttl registry, creating a new metric if none exist, and updating the last accessed time
func (r *Registry) GetCounter(metricName string, labels prometheus.Labels, help string, ttl time.Duration) (prometheus.Counter, error) {
	hash, labelNames := r.hashLabels(labels)
	vh, mh := r.get(metricName, hash, CounterMetricType)
	if mh != nil {
		return mh.(prometheus.Counter), nil
	}

	if r.metricConflicts(metricName, CounterMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}

	var counterVec *prometheus.CounterVec
	if vh == nil {
		r.metricsCount.WithLabelValues("counter").Inc()
		counterVec = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: metricName,
			Help: help,
		}, labelNames)

		if err := prometheus.Register(uncheckedCollector{counterVec}); err != nil {
			return nil, err
		}
	} else {
		counterVec = vh.(*prometheus.CounterVec)
	}

	var counter prometheus.Counter
	var err error
	if counter, err = counterVec.GetMetricWith(labels); err != nil {
		return nil, err
	}
	r.storeCounter(metricName, hash, labels, counterVec, counter, ttl)

	return counter, nil
}

// GetGauge gets a prometheus.Gauge from the ttl registry
func (r *Registry) GetGauge(metricName string, labels prometheus.Labels, help string, ttl time.Duration) (prometheus.Gauge, error) {
	hash, labelNames := r.hashLabels(labels)
	vh, mh := r.get(metricName, hash, GaugeMetricType)
	if mh != nil {
		return mh.(prometheus.Gauge), nil
	}

	if r.metricConflicts(metricName, GaugeMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}

	var gaugeVec *prometheus.GaugeVec
	if vh == nil {
		r.metricsCount.WithLabelValues("gauge").Inc()
		gaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: metricName,
			Help: help,
		}, labelNames)

		if err := prometheus.Register(uncheckedCollector{gaugeVec}); err != nil {
			return nil, err
		}
	} else {
		gaugeVec = vh.(*prometheus.GaugeVec)
	}

	var gauge prometheus.Gauge
	var err error
	if gauge, err = gaugeVec.GetMetricWith(labels); err != nil {
		return nil, err
	}
	r.storeGauge(metricName, hash, labels, gaugeVec, gauge, ttl)

	return gauge, nil
}

// GetHistogram gets a prometheus.Observer for a histogram from the ttl registry
func (r *Registry) GetHistogram(metricName string, labels prometheus.Labels, help string, buckets []float64, ttl time.Duration) (prometheus.Observer, error) {
	hash, labelNames := r.hashLabels(labels)
	vh, mh := r.get(metricName, hash, HistogramMetricType)
	if mh != nil {
		return mh.(prometheus.Observer), nil
	}

	if r.metricConflicts(metricName, HistogramMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}
	if r.metricConflicts(metricName+"_sum", HistogramMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}
	if r.metricConflicts(metricName+"_count", HistogramMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}
	if r.metricConflicts(metricName+"_bucket", HistogramMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}

	var histogramVec *prometheus.HistogramVec
	if vh == nil {
		r.metricsCount.WithLabelValues("histogram").Inc()
		if buckets == nil || len(buckets) == 0 {
			buckets = r.defaults.Buckets
		}
		histogramVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    metricName,
			Help:    help,
			Buckets: buckets,
		}, labelNames)

		if err := prometheus.Register(uncheckedCollector{histogramVec}); err != nil {
			return nil, err
		}
	} else {
		histogramVec = vh.(*prometheus.HistogramVec)
	}

	var observer prometheus.Observer
	var err error
	if observer, err = histogramVec.GetMetricWith(labels); err != nil {
		return nil, err
	}
	r.storeHistogram(metricName, hash, labels, histogramVec, observer, ttl)

	return observer, nil
}

// GetSummary gets a prometheus.Observer for a summary from the ttl registry
func (r *Registry) GetSummary(metricName string, labels prometheus.Labels, help string, objectives []mapper.MetricObjective, ttl time.Duration) (prometheus.Observer, error) {
	hash, labelNames := r.hashLabels(labels)
	vh, mh := r.get(metricName, hash, SummaryMetricType)
	if mh != nil {
		return mh.(prometheus.Observer), nil
	}

	if r.metricConflicts(metricName, SummaryMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}
	if r.metricConflicts(metricName+"_sum", SummaryMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}
	if r.metricConflicts(metricName+"_count", SummaryMetricType) {
		return nil, fmt.Errorf("metric with name %s is already registered", metricName)
	}

	var summaryVec *prometheus.SummaryVec
	if vh == nil {
		r.metricsCount.WithLabelValues("summary").Inc()
		// TODO: fix
		newQuantiles := r.defaults.Quantiles
		if objectives != nil && len(objectives) > 0 {
			newQuantiles = objectives
		}
		objectives := make(map[float64]float64)
		for _, q := range newQuantiles {
			objectives[q.Quantile] = q.Error
		}
		// In the case of no mapping file, explicitly define the default objectives
		if len(objectives) == 0 {
			objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
		}
		summaryVec = prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Name:       metricName,
			Help:       help,
			Objectives: objectives,
		}, labelNames)

		if err := prometheus.Register(uncheckedCollector{summaryVec}); err != nil {
			return nil, err
		}
	} else {
		summaryVec = vh.(*prometheus.SummaryVec)
	}

	var observer prometheus.Observer
	var err error
	if observer, err = summaryVec.GetMetricWith(labels); err != nil {
		return nil, err
	}
	r.storeSummary(metricName, hash, labels, summaryVec, observer, ttl)

	return observer, nil
}

// RemoveStaleMetrics removes expired metrics
func (r *Registry) RemoveStaleMetrics() {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	now := clock.Now()
	// delete timeseries with expired ttl
	for _, metric := range r.metrics {
		for hash, rm := range metric.metrics {
			if rm.ttl == 0 {
				continue
			}
			if rm.lastRegisteredAt.Add(rm.ttl).Before(now) {
				metric.vectors[rm.vecKey].holder.Delete(rm.labels)
				metric.vectors[rm.vecKey].refCount--
				delete(metric.metrics, hash)
			}
		}
	}
}

// Calculates a hash of both the label names and the label names and values.
func (r *Registry) hashLabels(labels prometheus.Labels) (labelHash, []string) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	r.hasher.Reset()
	r.nameBuf.Reset()
	r.valueBuf.Reset()
	labelNames := make([]string, 0, len(labels))

	for labelName := range labels {
		labelNames = append(labelNames, labelName)
	}
	sort.Strings(labelNames)

	r.valueBuf.WriteByte(model.SeparatorByte)
	for _, labelName := range labelNames {
		r.valueBuf.WriteString(labels[labelName])
		r.valueBuf.WriteByte(model.SeparatorByte)

		r.nameBuf.WriteString(labelName)
		r.nameBuf.WriteByte(model.SeparatorByte)
	}

	lh := labelHash{}
	r.hasher.Write(r.nameBuf.Bytes()) // nolint
	lh.names = nameHash(r.hasher.Sum64())

	// Now add the values to the names we've already hashed.
	r.hasher.Write(r.valueBuf.Bytes()) // nolint
	lh.values = valueHash(r.hasher.Sum64())

	return lh, labelNames
}
