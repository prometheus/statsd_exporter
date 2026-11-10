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

package registry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
	"github.com/prometheus/statsd_exporter/pkg/metrics"
)

func TestRegistryReset(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := &mapper.MetricMapper{}
	r := NewRegistry(reg, m)

	// Use Store directly with correct signature
	metric := metrics.Metric{
		MetricType: metrics.CounterMetricType,
		Vectors:    make(map[metrics.NameHash]*metrics.Vector),
		Metrics:    make(map[metrics.ValueHash]*metrics.RegisteredMetric),
	}
	r.Metrics["test_metric"] = metric

	if len(r.Metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(r.Metrics))
	}

	// Reset should clear all metrics
	r.Reset()

	if len(r.Metrics) != 0 {
		t.Fatalf("expected 0 metrics after reset, got %d", len(r.Metrics))
	}
}

func TestRegistryResetAfterStore(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := &mapper.MetricMapper{}
	r := NewRegistry(reg, m)

	// Simulate storing metrics with different types
	r.Metrics["metric1"] = metrics.Metric{
		MetricType: metrics.CounterMetricType,
		Vectors:    make(map[metrics.NameHash]*metrics.Vector),
		Metrics:    make(map[metrics.ValueHash]*metrics.RegisteredMetric),
	}
	r.Metrics["metric2"] = metrics.Metric{
		MetricType: metrics.GaugeMetricType,
		Vectors:    make(map[metrics.NameHash]*metrics.Vector),
		Metrics:    make(map[metrics.ValueHash]*metrics.RegisteredMetric),
	}

	if len(r.Metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(r.Metrics))
	}

	r.Reset()

	if len(r.Metrics) != 0 {
		t.Fatalf("expected 0 metrics after reset, got %d", len(r.Metrics))
	}
}
