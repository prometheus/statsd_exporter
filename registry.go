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

package main

import (
	"bytes"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

type metricType int

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

type registry struct {
	metrics map[string]metric
	mapper  *mapper.MetricMapper
	// The below value and label variables are allocated in the registry struct
	// so that we don't have to allocate them every time have to compute a label
	// hash.
	valueBuf, nameBuf bytes.Buffer
	hasher            hash.Hash64
}

func newRegistry(mapper *mapper.MetricMapper) *registry {
	return &registry{
		metrics: make(map[string]metric),
		mapper:  mapper,
		hasher:  fnv.New64a(),
	}
}

func (r *registry) metricConflicts(metricName string, metricType metricType) bool {
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

func (r *registry) storeCounter(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.CounterVec, c prometheus.Counter, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, c, CounterMetricType, ttl)
}

func (r *registry) storeGauge(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.GaugeVec, g prometheus.Counter, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, g, GaugeMetricType, ttl)
}

func (r *registry) storeHistogram(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.HistogramVec, o prometheus.Observer, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, o, HistogramMetricType, ttl)
}

func (r *registry) storeSummary(metricName string, hash labelHash, labels prometheus.Labels, vec *prometheus.SummaryVec, o prometheus.Observer, ttl time.Duration) {
	r.store(metricName, hash, labels, vec, o, SummaryMetricType, ttl)
}

func (r *registry) store(metricName string, hash labelHash, labels prometheus.Labels, vh vectorHolder, mh metricHolder, metricType metricType, ttl time.Duration) {
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

func (r *registry) get(metricName string, hash labelHash, metricType metricType) (vectorHolder, metricHolder) {
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

func (r *registry) getCounter(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping) (prometheus.Counter, error) {
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
		metricsCount.WithLabelValues("counter").Inc()
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
	r.storeCounter(metricName, hash, labels, counterVec, counter, mapping.Ttl)

	return counter, nil
}

func (r *registry) getGauge(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping) (prometheus.Gauge, error) {
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
		metricsCount.WithLabelValues("gauge").Inc()
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
	r.storeGauge(metricName, hash, labels, gaugeVec, gauge, mapping.Ttl)

	return gauge, nil
}

func (r *registry) getHistogram(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping) (prometheus.Observer, error) {
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
		metricsCount.WithLabelValues("histogram").Inc()
		buckets := r.mapper.Defaults.Buckets
		if mapping.HistogramOptions != nil && len(mapping.HistogramOptions.Buckets) > 0 {
			buckets = mapping.HistogramOptions.Buckets
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
	r.storeHistogram(metricName, hash, labels, histogramVec, observer, mapping.Ttl)

	return observer, nil
}

func (r *registry) getSummary(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping) (prometheus.Observer, error) {
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
		metricsCount.WithLabelValues("summary").Inc()
		quantiles := r.mapper.Defaults.Quantiles
		if mapping != nil && mapping.SummaryOptions != nil && len(mapping.SummaryOptions.Quantiles) > 0 {
			quantiles = mapping.SummaryOptions.Quantiles
		}
		summaryOptions := mapper.SummaryOptions{}
		if mapping != nil && mapping.SummaryOptions != nil {
			summaryOptions = *mapping.SummaryOptions
		}
		objectives := make(map[float64]float64)
		for _, q := range quantiles {
			objectives[q.Quantile] = q.Error
		}
		// In the case of no mapping file, explicitly define the default quantiles
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
	r.storeSummary(metricName, hash, labels, summaryVec, observer, mapping.Ttl)

	return observer, nil
}

func (r *registry) removeStaleMetrics() {
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
func (r *registry) hashLabels(labels prometheus.Labels) (labelHash, []string) {
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
	r.hasher.Write(r.nameBuf.Bytes())
	lh.names = nameHash(r.hasher.Sum64())

	// Now add the values to the names we've already hashed.
	r.hasher.Write(r.valueBuf.Bytes())
	lh.values = valueHash(r.hasher.Sum64())

	return lh, labelNames
}
