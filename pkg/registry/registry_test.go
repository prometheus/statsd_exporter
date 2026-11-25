// Copyright The Prometheus Authors
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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
	"github.com/prometheus/statsd_exporter/pkg/metrics"
)

func newTestRegistry() (*Registry, *prometheus.Registry, *prometheus.GaugeVec) {
	reg := prometheus.NewRegistry()
	metricsCount := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "metrics_count"}, []string{"type"})
	return NewRegistry(reg, &mapper.MetricMapper{}), reg, metricsCount
}

// countSeries returns the number of series gathered from reg.
func countSeries(t *testing.T, reg *prometheus.Registry) int {
	t.Helper()
	mfs, err := reg.Gather()
	assertNoErr(t, err)
	n := 0
	for _, mf := range mfs {
		n += len(mf.GetMetric())
	}
	return n
}

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestClearMetrics(t *testing.T) {
	r, reg, metricsCount := newTestRegistry()
	mapping := &mapper.MetricMapping{}

	for _, labels := range []prometheus.Labels{{"env": "a"}, {"env": "b"}} {
		c, err := r.GetCounter("clear_counter", labels, "help", mapping, metricsCount)
		assertNoErr(t, err)
		c.Inc()
		g, err := r.GetGauge("clear_gauge", labels, "help", mapping, metricsCount)
		assertNoErr(t, err)
		g.Set(1)
		h, err := r.GetHistogram("clear_histogram", labels, "help", mapping, metricsCount)
		assertNoErr(t, err)
		h.Observe(1)
		s, err := r.GetSummary("clear_summary", labels, "help", mapping, metricsCount)
		assertNoErr(t, err)
		s.Observe(1)
	}
	// One metric name with two label-name schemas, so two vectors.
	for _, labels := range []prometheus.Labels{{"a": "1"}, {"b": "1"}} {
		g, err := r.GetGauge("clear_two_schemas", labels, "help", mapping, metricsCount)
		assertNoErr(t, err)
		g.Set(1)
	}
	if n := len(r.Metrics["clear_two_schemas"].Vectors); n != 2 {
		t.Fatalf("expected 2 vectors for clear_two_schemas, got %d", n)
	}

	want := 0
	for _, metric := range r.Metrics {
		want += len(metric.Metrics)
	}
	if want != 10 {
		t.Fatalf("expected 10 registered series before clear, got %d", want)
	}
	if n := countSeries(t, reg); n != want {
		t.Fatalf("expected %d gathered series before clear, got %d", want, n)
	}

	if cleared := r.ClearMetrics(); cleared != want {
		t.Fatalf("expected ClearMetrics to return %d, got %d", want, cleared)
	}

	for name, metric := range r.Metrics {
		if len(metric.Metrics) != 0 {
			t.Errorf("%s: expected no registered series, got %d", name, len(metric.Metrics))
		}
		for _, v := range metric.Vectors {
			if v.RefCount != 0 {
				t.Errorf("%s: expected RefCount 0, got %d", name, v.RefCount)
			}
		}
	}
	if n := countSeries(t, reg); n != 0 {
		t.Fatalf("expected no gathered series after clear, got %d", n)
	}
}

func TestClearMetricsRecreatesWithFreshState(t *testing.T) {
	r, reg, metricsCount := newTestRegistry()
	mapping := &mapper.MetricMapping{}
	labels := prometheus.Labels{"env": "a"}

	c, err := r.GetCounter("recreate_counter", labels, "help", mapping, metricsCount)
	assertNoErr(t, err)
	c.Add(5)

	r.ClearMetrics()

	c, err = r.GetCounter("recreate_counter", labels, "help", mapping, metricsCount)
	assertNoErr(t, err)
	c.Inc()

	m := &dto.Metric{}
	assertNoErr(t, c.Write(m))
	if got := m.GetCounter().GetValue(); got != 1 {
		t.Fatalf("expected recreated counter to start from 0 and be 1, got %v", got)
	}
	if n := countSeries(t, reg); n != 1 {
		t.Fatalf("expected 1 gathered series after recreate, got %d", n)
	}
	if v := r.Metrics["recreate_counter"].Vectors; len(v) != 1 {
		t.Fatalf("expected the existing vector to be reused, got %d vectors", len(v))
	}
}

func TestClearMetricsKeepsMetricType(t *testing.T) {
	r, _, metricsCount := newTestRegistry()
	mapping := &mapper.MetricMapping{}

	_, err := r.GetGauge("foo", prometheus.Labels{}, "help", mapping, metricsCount)
	assertNoErr(t, err)

	r.ClearMetrics()

	if _, err := r.GetCounter("foo", prometheus.Labels{}, "help", mapping, metricsCount); err == nil {
		t.Fatal("expected a counter named foo to be rejected after clear, since foo is still a gauge")
	}
	if _, err := r.GetGauge("foo", prometheus.Labels{}, "help", mapping, metricsCount); err != nil {
		t.Fatalf("expected gauge foo to be recreated after clear: %v", err)
	}
}

func TestRemoveStaleMetricsRefCountGuard(t *testing.T) {
	clockInstance := clock.ClockInstance
	defer func() {
		clock.ClockInstance = clockInstance
	}()
	clock.ClockInstance = &clock.Clock{Instant: time.Unix(0, 0)}

	r, _, metricsCount := newTestRegistry()
	_, err := r.GetGauge("ttl_gauge", prometheus.Labels{}, "help", &mapper.MetricMapping{Ttl: time.Second}, metricsCount)
	assertNoErr(t, err)

	metric := r.Metrics["ttl_gauge"]
	if len(metric.Vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(metric.Vectors))
	}
	for _, v := range metric.Vectors {
		// Force the inconsistent state the guard protects against.
		v.RefCount = 0
	}

	clock.ClockInstance.Instant = time.Unix(2, 0)
	r.RemoveStaleMetrics()

	if len(metric.Metrics) != 0 {
		t.Fatalf("expected the expired series to be removed, got %d", len(metric.Metrics))
	}
	for _, v := range metric.Vectors {
		if v.RefCount != 0 {
			t.Fatalf("expected RefCount to stay 0, got %d", v.RefCount)
		}
	}
}

// deleteOnlyHolder is a VectorHolder without Reset.
type deleteOnlyHolder struct{ deleted int }

func (h *deleteOnlyHolder) Delete(prometheus.Labels) bool {
	h.deleted++
	return true
}

func TestClearMetricsHolderWithoutReset(t *testing.T) {
	r, _, _ := newTestRegistry()
	holder := &deleteOnlyHolder{}
	for _, value := range []string{"a", "b"} {
		labels := prometheus.Labels{"env": value}
		hash, _ := r.HashLabels(labels)
		r.Store("delete_only", hash, labels, holder, nil, metrics.GaugeMetricType, 0)
	}

	if cleared := r.ClearMetrics(); cleared != 2 {
		t.Fatalf("expected ClearMetrics to return 2, got %d", cleared)
	}
	if holder.deleted != 2 {
		t.Fatalf("expected 2 per-series deletes, got %d", holder.deleted)
	}
	metric := r.Metrics["delete_only"]
	if len(metric.Metrics) != 0 {
		t.Fatalf("expected no registered series, got %d", len(metric.Metrics))
	}
	for _, v := range metric.Vectors {
		if v.RefCount != 0 {
			t.Fatalf("expected RefCount 0, got %d", v.RefCount)
		}
	}
}

// activeVectors reads the active-vectors gauge for one metric type.
func activeVectors(t *testing.T, g *prometheus.GaugeVec, typ string) float64 {
	t.Helper()
	var m dto.Metric
	assertNoErr(t, g.WithLabelValues(typ).Write(&m))
	return m.GetGauge().GetValue()
}

func TestActiveVectorsGauge(t *testing.T) {
	clockInstance := clock.ClockInstance
	defer func() {
		clock.ClockInstance = clockInstance
	}()
	clock.ClockInstance = &clock.Clock{Instant: time.Unix(0, 0)}

	r, _, metricsCount := newTestRegistry()
	active := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "active_vectors"}, []string{"type"})
	r.SetActiveVectorsGauge(active)
	ttl := &mapper.MetricMapping{Ttl: time.Second}

	// Two series of one vector count once; another label set is another vector.
	for _, labels := range []prometheus.Labels{{"env": "a"}, {"env": "b"}, {"env": "a", "zone": "x"}} {
		_, err := r.GetCounter("active_counter", labels, "help", ttl, metricsCount)
		assertNoErr(t, err)
	}
	_, err := r.GetGauge("active_gauge", prometheus.Labels{}, "help", ttl, metricsCount)
	assertNoErr(t, err)
	_, err = r.GetHistogram("active_histogram", prometheus.Labels{}, "help", ttl, metricsCount)
	assertNoErr(t, err)
	_, err = r.GetSummary("active_summary", prometheus.Labels{}, "help", ttl, metricsCount)
	assertNoErr(t, err)

	for typ, want := range map[string]float64{"counter": 2, "gauge": 1, "histogram": 1, "summary": 1} {
		if got := activeVectors(t, active, typ); got != want {
			t.Fatalf("after registration: %s = %v, want %v", typ, got, want)
		}
	}

	// Expiry of every series brings the gauge back to zero.
	clock.ClockInstance.Instant = time.Unix(2, 0)
	r.RemoveStaleMetrics()
	for _, typ := range []string{"counter", "gauge", "histogram", "summary"} {
		if got := activeVectors(t, active, typ); got != 0 {
			t.Fatalf("after expiry: %s = %v, want 0", typ, got)
		}
	}

	// The same vector becoming active again counts again.
	_, err = r.GetCounter("active_counter", prometheus.Labels{"env": "a"}, "help", ttl, metricsCount)
	assertNoErr(t, err)
	if got := activeVectors(t, active, "counter"); got != 1 {
		t.Fatalf("after re-registration: counter = %v, want 1", got)
	}

	// A vector stays active while one of its series is alive.
	clock.ClockInstance.Instant = time.Unix(3, 0)
	_, err = r.GetCounter("active_counter", prometheus.Labels{"env": "b"}, "help", ttl, metricsCount)
	assertNoErr(t, err)
	clock.ClockInstance.Instant = time.Unix(3, 500)
	r.RemoveStaleMetrics() // env=a expired, env=b did not
	if got := activeVectors(t, active, "counter"); got != 1 {
		t.Fatalf("with one live series: counter = %v, want 1", got)
	}

	// Clearing the registry deactivates every vector.
	_, err = r.GetGauge("active_gauge", prometheus.Labels{}, "help", ttl, metricsCount)
	assertNoErr(t, err)
	r.ClearMetrics()
	for _, typ := range []string{"counter", "gauge"} {
		if got := activeVectors(t, active, typ); got != 0 {
			t.Fatalf("after clear: %s = %v, want 0", typ, got)
		}
	}
}

func TestActiveVectorsGaugeHolderWithoutReset(t *testing.T) {
	r, _, _ := newTestRegistry()
	active := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "active_vectors"}, []string{"type"})
	r.SetActiveVectorsGauge(active)

	holder := &deleteOnlyHolder{}
	for _, value := range []string{"a", "b"} {
		labels := prometheus.Labels{"env": value}
		hash, _ := r.HashLabels(labels)
		r.Store("custom", hash, labels, holder, struct{}{}, metrics.GaugeMetricType, 0)
	}
	if got := activeVectors(t, active, "gauge"); got != 1 {
		t.Fatalf("after registration: gauge = %v, want 1", got)
	}
	r.ClearMetrics()
	if got := activeVectors(t, active, "gauge"); got != 0 {
		t.Fatalf("after clear: gauge = %v, want 0", got)
	}
}
