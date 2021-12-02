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

package exporter

import (
	"os"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/event"
	"github.com/prometheus/statsd_exporter/pkg/level"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
	"github.com/prometheus/statsd_exporter/pkg/registry"
)

const (
	defaultHelp = "Metric autogenerated by statsd_exporter."
	regErrF     = "Failed to update metric"
)

type Registry interface {
	GetCounter(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping, metricsCount *prometheus.GaugeVec) (prometheus.Counter, error)
	GetGauge(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping, metricsCount *prometheus.GaugeVec) (prometheus.Gauge, error)
	GetHistogram(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping, metricsCount *prometheus.GaugeVec) (prometheus.Observer, error)
	GetSummary(metricName string, labels prometheus.Labels, help string, mapping *mapper.MetricMapping, metricsCount *prometheus.GaugeVec) (prometheus.Observer, error)
	RemoveStaleMetrics()
}

type Exporter struct {
	Mapper                *mapper.MetricMapper
	Registry              Registry
	Logger                log.Logger
	EventsActions         *prometheus.CounterVec
	EventsUnmapped        prometheus.Counter
	ErrorEventStats       *prometheus.CounterVec
	EventStats            *prometheus.CounterVec
	ConflictingEventStats *prometheus.CounterVec
	MetricsCount          *prometheus.GaugeVec
}

// Listen handles all events sent to the given channel sequentially. It
// terminates when the channel is closed.
func (b *Exporter) Listen(e <-chan event.Events) {
	removeStaleMetricsTicker := clock.NewTicker(time.Second)

	for {
		select {
		case <-removeStaleMetricsTicker.C:
			b.Registry.RemoveStaleMetrics()
		case events, ok := <-e:
			if !ok {
				level.Debug(b.Logger).Log("msg", "Channel is closed. Break out of Exporter.Listener.")
				removeStaleMetricsTicker.Stop()
				return
			}
			for _, event := range events {
				if b.Mapper.GlobalLabels != nil {
					for k, v := range b.Mapper.GlobalLabels {
						event.Labels()[k] = v
					}
				}
				b.handleEvent(event)
			}
		}
	}
}

// handleEvent processes a single Event according to the configured mapping.
func (b *Exporter) handleEvent(thisEvent event.Event) {
	mapping, labels, present := b.Mapper.GetMapping(thisEvent.MetricName(), thisEvent.MetricType())
	if mapping == nil {
		mapping = &mapper.MetricMapping{}
		if b.Mapper.Defaults.Ttl != 0 {
			mapping.Ttl = b.Mapper.Defaults.Ttl
		}
	}

	if mapping.Action == mapper.ActionTypeDrop {
		b.EventsActions.WithLabelValues("drop").Inc()
		return
	}

	metricName := ""

	help := defaultHelp
	if mapping.HelpText != "" {
		help = mapping.HelpText
	}

	prometheusLabels := thisEvent.Labels()
	if present {
		if mapping.Name == "" {
			level.Debug(b.Logger).Log("msg", "The mapping generates an empty metric name", "metric_name", thisEvent.MetricName(), "match", mapping.Match)
			b.ErrorEventStats.WithLabelValues("empty_metric_name").Inc()
			return
		}
		metricName = mapper.EscapeMetricName(mapping.Name)
		for label, value := range labels {
			prometheusLabels[label] = value
		}
		b.EventsActions.WithLabelValues(string(mapping.Action)).Inc()
	} else {
		b.EventsUnmapped.Inc()
		metricName = mapper.EscapeMetricName(thisEvent.MetricName())
	}

	switch ev := thisEvent.(type) {
	case *event.CounterEvent:
		// We don't accept negative values for counters. Incrementing the counter with a negative number
		// will cause the exporter to panic. Instead we will warn and continue to the next event.
		if thisEvent.Value() < 0.0 {
			level.Debug(b.Logger).Log("msg", "counter must be non-negative value", "metric", metricName, "event_value", thisEvent.Value())
			b.ErrorEventStats.WithLabelValues("illegal_negative_counter").Inc()
			return
		}

		counter, err := b.Registry.GetCounter(metricName, prometheusLabels, help, mapping, b.MetricsCount)
		if err == nil {
			counter.Add(thisEvent.Value())
			b.EventStats.WithLabelValues("counter").Inc()
		} else {
			level.Debug(b.Logger).Log("msg", regErrF, "metric", metricName, "error", err)
			b.ConflictingEventStats.WithLabelValues("counter").Inc()
		}

	case *event.GaugeEvent:
		gauge, err := b.Registry.GetGauge(metricName, prometheusLabels, help, mapping, b.MetricsCount)

		if err == nil {
			if ev.GRelative {
				gauge.Add(thisEvent.Value())
			} else {
				gauge.Set(thisEvent.Value())
			}
			b.EventStats.WithLabelValues("gauge").Inc()
		} else {
			level.Debug(b.Logger).Log("msg", regErrF, "metric", metricName, "error", err)
			b.ConflictingEventStats.WithLabelValues("gauge").Inc()
		}

	case *event.ObserverEvent:
		t := mapper.ObserverTypeDefault
		if mapping != nil {
			t = mapping.ObserverType
		}
		if t == mapper.ObserverTypeDefault {
			t = b.Mapper.Defaults.ObserverType
		}

		switch t {
		case mapper.ObserverTypeHistogram:
			histogram, err := b.Registry.GetHistogram(metricName, prometheusLabels, help, mapping, b.MetricsCount)
			if err == nil {
				histogram.Observe(thisEvent.Value())
				b.EventStats.WithLabelValues("observer").Inc()
			} else {
				level.Debug(b.Logger).Log("msg", regErrF, "metric", metricName, "error", err)
				b.ConflictingEventStats.WithLabelValues("observer").Inc()
			}

		case mapper.ObserverTypeDefault, mapper.ObserverTypeSummary:
			summary, err := b.Registry.GetSummary(metricName, prometheusLabels, help, mapping, b.MetricsCount)
			if err == nil {
				summary.Observe(thisEvent.Value())
				b.EventStats.WithLabelValues("observer").Inc()
			} else {
				level.Debug(b.Logger).Log("msg", regErrF, "metric", metricName, "error", err)
				b.ConflictingEventStats.WithLabelValues("observer").Inc()
			}

		default:
			level.Error(b.Logger).Log("msg", "unknown observer type", "type", t)
			os.Exit(1)
		}

	default:
		level.Debug(b.Logger).Log("msg", "Unsupported event type")
		b.EventStats.WithLabelValues("illegal").Inc()
	}
}

func NewExporter(reg prometheus.Registerer, mapper *mapper.MetricMapper, logger log.Logger, eventsActions *prometheus.CounterVec, eventsUnmapped prometheus.Counter, errorEventStats *prometheus.CounterVec, eventStats *prometheus.CounterVec, conflictingEventStats *prometheus.CounterVec, metricsCount *prometheus.GaugeVec) *Exporter {
	return &Exporter{
		Mapper:                mapper,
		Registry:              registry.NewRegistry(reg, mapper),
		Logger:                logger,
		EventsActions:         eventsActions,
		EventsUnmapped:        eventsUnmapped,
		ErrorEventStats:       errorEventStats,
		EventStats:            eventStats,
		ConflictingEventStats: conflictingEventStats,
		MetricsCount:          metricsCount,
	}
}
