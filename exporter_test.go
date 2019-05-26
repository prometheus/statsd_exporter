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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

// TestNegativeCounter validates when we send a negative
// number to a counter that we no longer panic the Exporter Listener.
func TestNegativeCounter(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			err := e.(error)
			if err.Error() == "counter cannot decrease in value" {
				t.Fatalf("Counter was negative and causes a panic.")
			} else {
				t.Fatalf("Unknown panic and error: %q", err.Error())
			}
		}
	}()

	events := make(chan Events)
	go func() {
		c := Events{
			&CounterEvent{
				metricName: "foo",
				value:      -1,
			},
		}
		events <- c
		close(events)
	}()

	errorCounter := errorEventStats.WithLabelValues("illegal_negative_counter")
	prev := getTelemetryCounterValue(errorCounter)

	testMapper := mapper.MetricMapper{}
	testMapper.InitCache(0)

	ex := NewExporter(&testMapper)
	ex.Listen(events)

	updated := getTelemetryCounterValue(errorCounter)
	if updated-prev != 1 {
		t.Fatal("Illegal negative counter error not counted")
	}
}

// TestInconsistentLabelSets validates that the exporter will register
// and record metrics with the same metric name but inconsistent label
// sets e.g foo{a="1"} and foo{b="1"}
func TestInconsistentLabelSets(t *testing.T) {
	firstLabelSet := make(map[string]string)
	secondLabelSet := make(map[string]string)
	metricNames := [4]string{"counter_test", "gauge_test", "histogram_test", "summary_test"}

	firstLabelSet["foo"] = "1"
	secondLabelSet["foo"] = "1"
	secondLabelSet["bar"] = "2"

	events := make(chan Events)
	go func() {
		c := Events{
			&CounterEvent{
				metricName: "counter_test",
				value:      1,
				labels:     firstLabelSet,
			},
			&CounterEvent{
				metricName: "counter_test",
				value:      1,
				labels:     secondLabelSet,
			},
			&GaugeEvent{
				metricName: "gauge_test",
				value:      1,
				labels:     firstLabelSet,
			},
			&GaugeEvent{
				metricName: "gauge_test",
				value:      1,
				labels:     secondLabelSet,
			},
			&TimerEvent{
				metricName: "histogram.test",
				value:      1,
				labels:     firstLabelSet,
			},
			&TimerEvent{
				metricName: "histogram.test",
				value:      1,
				labels:     secondLabelSet,
			},
			&TimerEvent{
				metricName: "summary_test",
				value:      1,
				labels:     firstLabelSet,
			},
			&TimerEvent{
				metricName: "summary_test",
				value:      1,
				labels:     secondLabelSet,
			},
		}
		events <- c
		close(events)
	}()

	config := `
mappings:
- match: histogram.test
  timer_type: histogram
  name: "histogram_test"
`
	testMapper := &mapper.MetricMapper{}
	err := testMapper.InitFromYAMLString(config, 0)
	if err != nil {
		t.Fatalf("Config load error: %s %s", config, err)
	}

	ex := NewExporter(testMapper)
	ex.Listen(events)

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
	}

	for _, metricName := range metricNames {
		firstMetric := getFloat64(metrics, metricName, firstLabelSet)
		secondMetric := getFloat64(metrics, metricName, secondLabelSet)

		if firstMetric == nil {
			t.Fatalf("Could not find time series with first label set for metric: %s", metricName)
		}
		if secondMetric == nil {
			t.Fatalf("Could not find time series with second label set for metric: %s", metricName)
		}
	}
}

// TestLabelParsing verifies that labels getting parsed out of metric
// names are being properly created.
func TestLabelParsing(t *testing.T) {
	codes := [2]string{"200", "300"}

	events := make(chan Events)
	go func() {
		c := Events{
			&CounterEvent{
				metricName: "counter.test.200",
				value:      1,
				labels:     make(map[string]string),
			},
			&CounterEvent{
				metricName: "counter.test.300",
				value:      1,
				labels:     make(map[string]string),
			},
		}
		events <- c
		close(events)
	}()

	config := `
mappings:
- match: counter.test.*
  name: "counter_test"
  labels:
    code: $1
`

	testMapper := &mapper.MetricMapper{}
	err := testMapper.InitFromYAMLString(config, 0)
	if err != nil {
		t.Fatalf("Config load error: %s %s", config, err)
	}

	ex := NewExporter(testMapper)
	ex.Listen(events)

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
	}

	labels := make(map[string]string)

	for _, code := range codes {
		labels["code"] = code
		if getFloat64(metrics, "counter_test", labels) == nil {
			t.Fatalf("Could not find metrics for counter_test code %s", code)
		}
	}
}

// TestConflictingMetrics validates that the exporter will not register metrics
// of different types that have overlapping names.
func TestConflictingMetrics(t *testing.T) {
	scenarios := []struct {
		name     string
		expected []float64
		in       Events
	}{
		{
			name:     "counter vs gauge",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "cvg_test",
					value:      1,
				},
				&GaugeEvent{
					metricName: "cvg_test",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs gauge with different labels",
			expected: []float64{1, 2},
			in: Events{
				&CounterEvent{
					metricName: "cvgl_test",
					value:      1,
					labels:     map[string]string{"tag": "1"},
				},
				&CounterEvent{
					metricName: "cvgl_test",
					value:      2,
					labels:     map[string]string{"tag": "2"},
				},
				&GaugeEvent{
					metricName: "cvgl_test",
					value:      3,
					labels:     map[string]string{"tag": "1"},
				},
			},
		},
		{
			name:     "counter vs gauge with same labels",
			expected: []float64{3},
			in: Events{
				&CounterEvent{
					metricName: "cvgsl_test",
					value:      1,
					labels:     map[string]string{"tag": "1"},
				},
				&CounterEvent{
					metricName: "cvgsl_test",
					value:      2,
					labels:     map[string]string{"tag": "1"},
				},
				&GaugeEvent{
					metricName: "cvgsl_test",
					value:      3,
					labels:     map[string]string{"tag": "1"},
				},
			},
		},
		{
			name:     "gauge vs counter",
			expected: []float64{2},
			in: Events{
				&GaugeEvent{
					metricName: "gvc_test",
					value:      2,
				},
				&CounterEvent{
					metricName: "gvc_test",
					value:      1,
				},
			},
		},
		{
			name:     "counter vs histogram",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "histogram_test1",
					value:      1,
				},
				&TimerEvent{
					metricName: "histogram.test1",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs histogram sum",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "histogram_test1_sum",
					value:      1,
				},
				&TimerEvent{
					metricName: "histogram.test1",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs histogram count",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "histogram_test2_count",
					value:      1,
				},
				&TimerEvent{
					metricName: "histogram.test2",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs histogram bucket",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "histogram_test3_bucket",
					value:      1,
				},
				&TimerEvent{
					metricName: "histogram.test3",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs summary quantile",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "cvsq_test",
					value:      1,
				},
				&TimerEvent{
					metricName: "cvsq_test",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs summary count",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "cvsc_count",
					value:      1,
				},
				&TimerEvent{
					metricName: "cvsc",
					value:      2,
				},
			},
		},
		{
			name:     "counter vs summary sum",
			expected: []float64{1},
			in: Events{
				&CounterEvent{
					metricName: "cvss_sum",
					value:      1,
				},
				&TimerEvent{
					metricName: "cvss",
					value:      2,
				},
			},
		},
	}

	config := `
mappings:
- match: histogram.*
  timer_type: histogram
  name: "histogram_${1}"
`
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			testMapper := &mapper.MetricMapper{}
			err := testMapper.InitFromYAMLString(config, 0)
			if err != nil {
				t.Fatalf("Config load error: %s %s", config, err)
			}

			events := make(chan Events)
			go func() {
				events <- s.in
				close(events)
			}()
			ex := NewExporter(testMapper)
			ex.Listen(events)

			metrics, err := prometheus.DefaultGatherer.Gather()
			if err != nil {
				t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
			}

			for i, e := range s.expected {
				mn := s.in[i].MetricName()
				m := getFloat64(metrics, mn, s.in[i].Labels())

				if m == nil {
					t.Fatalf("Could not find time series with metric name '%v'", mn)
				}

				if *m != e {
					t.Fatalf("Expected to get %v, but got %v instead", e, *m)
				}
			}
		})
	}
}

// TestEmptyStringMetric validates when a metric name ends up
// being the empty string after applying the match replacements
// tha we don't panic the Exporter Listener.
func TestEmptyStringMetric(t *testing.T) {
	events := make(chan Events)
	go func() {
		c := Events{
			&CounterEvent{
				metricName: "foo_bar",
				value:      1,
			},
		}
		events <- c
		close(events)
	}()

	config := `
mappings:
- match: .*_bar
  match_type: regex
  name: "${1}"
`
	testMapper := &mapper.MetricMapper{}
	err := testMapper.InitFromYAMLString(config, 0)
	if err != nil {
		t.Fatalf("Config load error: %s %s", config, err)
	}

	errorCounter := errorEventStats.WithLabelValues("empty_metric_name")
	prev := getTelemetryCounterValue(errorCounter)

	ex := NewExporter(testMapper)
	ex.Listen(events)

	updated := getTelemetryCounterValue(errorCounter)
	if updated-prev != 1 {
		t.Fatal("Empty metric name error event not counted")
	}
}

// TestInvalidUtf8InDatadogTagValue validates robustness of exporter listener
// against datadog tags with invalid tag values.
// It sends the same tags first with a valid value, then with an invalid one.
// The exporter should not panic, but drop the invalid event
func TestInvalidUtf8InDatadogTagValue(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			err := e.(error)
			t.Fatalf("Exporter listener should not panic on bad utf8: %q", err.Error())
		}
	}()

	events := make(chan Events)
	ueh := &unbufferedEventHandler{c: events}

	go func() {
		for _, l := range []statsDPacketHandler{&StatsDUDPListener{}, &mockStatsDTCPListener{}} {
			l.SetEventHandler(ueh)
			l.handlePacket([]byte("bar:200|c|#tag:value\nbar:200|c|#tag:\xc3\x28invalid"))
		}
		close(events)
	}()

	testMapper := mapper.MetricMapper{}
	testMapper.InitCache(0)

	ex := NewExporter(&testMapper)
	ex.Listen(events)
}

// In the case of someone starting the statsd exporter with no mapping file specified
// which is valid, we want to make sure that the default quantile metrics are generated
// as well as the sum/count metrics
func TestSummaryWithQuantilesEmptyMapping(t *testing.T) {
	// Start exporter with a synchronous channel
	events := make(chan Events)
	go func() {
		testMapper := mapper.MetricMapper{}
		testMapper.InitCache(0)

		ex := NewExporter(&testMapper)
		ex.Listen(events)
	}()

	name := "default_foo"
	c := Events{
		&TimerEvent{
			metricName: name,
			value:      300,
		},
	}
	events <- c
	events <- Events{}
	close(events)

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal("Gather should not fail: ", err)
	}

	var metricFamily *dto.MetricFamily
	for _, m := range metrics {
		if *m.Name == name {
			metricFamily = m
			break
		}
	}

	if metricFamily == nil {
		t.Fatal("Metric could not be found")
	}

	quantiles := metricFamily.Metric[0].Summary.Quantile
	if len(quantiles) == 0 {
		t.Fatal("Summary has no quantiles available")
	}
}

func TestHistogramUnits(t *testing.T) {
	// Start exporter with a synchronous channel
	events := make(chan Events)
	go func() {
		testMapper := mapper.MetricMapper{}
		testMapper.InitCache(0)
		ex := NewExporter(&testMapper)
		ex.mapper.Defaults.TimerType = mapper.TimerTypeHistogram
		ex.Listen(events)
	}()

	// Synchronously send a statsd event to wait for handleEvent execution.
	// Then close events channel to stop a listener.
	name := "foo"
	c := Events{
		&TimerEvent{
			metricName: name,
			value:      300,
		},
	}
	events <- c
	events <- Events{}
	close(events)

	// Check histogram value
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
	}
	value := getFloat64(metrics, name, prometheus.Labels{})
	if value == nil {
		t.Fatal("Histogram value should not be nil")
	}
	if *value == 300 {
		t.Fatalf("Histogram observations not scaled into Seconds")
	} else if *value != .300 {
		t.Fatalf("Received unexpected value for histogram observation %f != .300", *value)
	}
}
func TestCounterIncrement(t *testing.T) {
	// Start exporter with a synchronous channel
	events := make(chan Events)
	go func() {
		testMapper := mapper.MetricMapper{}
		testMapper.InitCache(0)
		ex := NewExporter(&testMapper)
		ex.Listen(events)
	}()

	// Synchronously send a statsd event to wait for handleEvent execution.
	// Then close events channel to stop a listener.
	name := "foo_counter"
	labels := map[string]string{
		"foo": "bar",
	}
	c := Events{
		&CounterEvent{
			metricName: name,
			value:      1,
			labels:     labels,
		},
		&CounterEvent{
			metricName: name,
			value:      1,
			labels:     labels,
		},
	}
	events <- c
	// Push empty event so that we block until the first event is consumed.
	events <- Events{}
	close(events)

	// Check histogram value
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
	}
	value := getFloat64(metrics, name, labels)
	if value == nil {
		t.Fatal("Counter value should not be nil")
	}
	if *value != 2 {
		t.Fatalf("Counter wasn't incremented properly")
	}
}

type statsDPacketHandler interface {
	handlePacket(packet []byte)
	SetEventHandler(eh eventHandler)
}

type mockStatsDTCPListener struct {
	StatsDTCPListener
}

func (ml *mockStatsDTCPListener) handlePacket(packet []byte) {
	// Forcing IPv4 because the TravisCI build environment does not have IPv6
	// addresses.
	lc, err := net.ListenTCP("tcp4", nil)
	if err != nil {
		panic(fmt.Sprintf("mockStatsDTCPListener: listen failed: %v", err))
	}

	defer lc.Close()

	go func() {
		cc, err := net.DialTCP("tcp", nil, lc.Addr().(*net.TCPAddr))
		if err != nil {
			panic(fmt.Sprintf("mockStatsDTCPListener: dial failed: %v", err))
		}

		defer cc.Close()

		n, err := cc.Write(packet)
		if err != nil || n != len(packet) {
			panic(fmt.Sprintf("mockStatsDTCPListener: write failed: %v,%d", err, n))
		}
	}()

	sc, err := lc.AcceptTCP()
	if err != nil {
		panic(fmt.Sprintf("mockStatsDTCPListener: accept failed: %v", err))
	}
	ml.handleConn(sc)
}

func TestEscapeMetricName(t *testing.T) {
	scenarios := map[string]string{
		"clean":                   "clean",
		"0starts_with_digit":      "_0starts_with_digit",
		"with_underscore":         "with_underscore",
		"with.dot":                "with_dot",
		"with😱emoji":              "with_emoji",
		"with.*.multiple":         "with___multiple",
		"test.web-server.foo.bar": "test_web_server_foo_bar",
		"":                        "",
	}

	for in, want := range scenarios {
		if got := escapeMetricName(in); want != got {
			t.Errorf("expected `%s` to be escaped to `%s`, got `%s`", in, want, got)
		}
	}
}

// TestTtlExpiration validates expiration of time series.
// foobar metric without mapping should expire with default ttl of 1s
// bazqux metric should expire with ttl of 2s
func TestTtlExpiration(t *testing.T) {
	// Mock a time.NewTicker
	tickerCh := make(chan time.Time)
	clock.ClockInstance = &clock.Clock{
		TickerCh: tickerCh,
	}

	config := `
defaults:
  ttl: 1s
mappings:
- match: bazqux.*
  name: bazqux
  ttl: 2s
`
	// Create mapper from config and start an Exporter with a synchronous channel
	testMapper := &mapper.MetricMapper{}
	err := testMapper.InitFromYAMLString(config, 0)
	if err != nil {
		t.Fatalf("Config load error: %s %s", config, err)
	}
	events := make(chan Events)
	defer close(events)
	go func() {
		ex := NewExporter(testMapper)
		ex.Listen(events)
	}()

	ev := Events{
		// event with default ttl = 1s
		&GaugeEvent{
			metricName: "foobar",
			value:      200,
		},
		// event with ttl = 2s from a mapping
		&TimerEvent{
			metricName: "bazqux.main",
			value:      42000,
		},
	}

	var metrics []*dto.MetricFamily
	var foobarValue *float64
	var bazquxValue *float64

	// Step 1. Send events with statsd metrics.
	// Send empty Events to wait for events are handled.
	// saveLabelValues will use fake instant as a lastRegisteredAt time.
	clock.ClockInstance.Instant = time.Unix(0, 0)
	events <- ev
	events <- Events{}

	// Check values
	metrics, err = prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal("Gather should not fail")
	}
	foobarValue = getFloat64(metrics, "foobar", prometheus.Labels{})
	bazquxValue = getFloat64(metrics, "bazqux", prometheus.Labels{})
	if foobarValue == nil || bazquxValue == nil {
		t.Fatalf("Gauge `foobar` and Summary `bazqux` should be gathered")
	}
	if *foobarValue != 200 {
		t.Fatalf("Gauge `foobar` observation %f is not expected. Should be 200", *foobarValue)
	}
	if *bazquxValue != 42 {
		t.Fatalf("Summary `bazqux` observation %f is not expected. Should be 42", *bazquxValue)
	}

	// Step 2. Increase Instant to emulate metrics expiration after 1s
	clock.ClockInstance.Instant = time.Unix(1, 10)
	clock.ClockInstance.TickerCh <- time.Unix(0, 0)
	events <- Events{}

	// Check values
	metrics, err = prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal("Gather should not fail")
	}
	foobarValue = getFloat64(metrics, "foobar", prometheus.Labels{})
	bazquxValue = getFloat64(metrics, "bazqux", prometheus.Labels{})
	if foobarValue != nil {
		t.Fatalf("Gauge `foobar` should be expired")
	}
	if bazquxValue == nil {
		t.Fatalf("Summary `bazqux` should be gathered")
	}
	if *bazquxValue != 42 {
		t.Fatalf("Summary `bazqux` observation %f is not expected. Should be 42", *bazquxValue)
	}

	// Step 3. Increase Instant to emulate metrics expiration after 2s
	clock.ClockInstance.Instant = time.Unix(2, 200)
	clock.ClockInstance.TickerCh <- time.Unix(0, 0)
	events <- Events{}

	// Check values
	metrics, err = prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal("Gather should not fail")
	}
	foobarValue = getFloat64(metrics, "foobar", prometheus.Labels{})
	bazquxValue = getFloat64(metrics, "bazqux", prometheus.Labels{})
	if bazquxValue != nil {
		t.Fatalf("Summary `bazqux` should be expired")
	}
	if foobarValue != nil {
		t.Fatalf("Gauge `foobar` should not be gathered after expiration")
	}
}

func TestHashLabelNames(t *testing.T) {
	r := newRegistry(nil)
	// Validate value hash changes and name has doesn't when just the value changes.
	hash1, _ := r.hashLabels(map[string]string{
		"label": "value1",
	})
	hash2, _ := r.hashLabels(map[string]string{
		"label": "value2",
	})
	if hash1.names != hash2.names {
		t.Fatal("Hash of label names should match, but doesn't")
	}
	if hash1.values == hash2.values {
		t.Fatal("Hash of label names shouldn't match, but do")
	}

	// Validate value and name hashes change when the name changes.
	hash1, _ = r.hashLabels(map[string]string{
		"label1": "value",
	})
	hash2, _ = r.hashLabels(map[string]string{
		"label2": "value",
	})
	if hash1.names == hash2.names {
		t.Fatal("Hash of label names shouldn't match, but do")
	}
	if hash1.values == hash2.values {
		t.Fatal("Hash of label names shouldn't match, but do")
	}
}

// getFloat64 search for metric by name in array of MetricFamily and then search a value by labels.
// Method returns a value or nil if metric is not found.
func getFloat64(metrics []*dto.MetricFamily, name string, labels prometheus.Labels) *float64 {
	var metricFamily *dto.MetricFamily
	for _, m := range metrics {
		if *m.Name == name {
			metricFamily = m
			break
		}
	}
	if metricFamily == nil {
		return nil
	}

	var metric *dto.Metric
	labelStr := fmt.Sprintf("%v", labels)
	for _, m := range metricFamily.Metric {
		l := labelPairsAsLabels(m.GetLabel())
		ls := fmt.Sprintf("%v", l)
		if labelStr == ls {
			metric = m
			break
		}
	}
	if metric == nil {
		return nil
	}

	var value float64
	if metric.Gauge != nil {
		value = metric.Gauge.GetValue()
		return &value
	}
	if metric.Counter != nil {
		value = metric.Counter.GetValue()
		return &value
	}
	if metric.Histogram != nil {
		value = metric.Histogram.GetSampleSum()
		return &value
	}
	if metric.Summary != nil {
		value = metric.Summary.GetSampleSum()
		return &value
	}
	if metric.Untyped != nil {
		value = metric.Untyped.GetValue()
		return &value
	}
	panic(fmt.Errorf("collected a non-gauge/counter/histogram/summary/untyped metric: %s", metric))
}

func labelPairsAsLabels(pairs []*dto.LabelPair) (labels prometheus.Labels) {
	labels = prometheus.Labels{}
	for _, pair := range pairs {
		if pair.Name == nil {
			continue
		}
		value := ""
		if pair.Value != nil {
			value = *pair.Value
		}
		labels[*pair.Name] = value
	}
	return
}

func getTelemetryCounterValue(counter prometheus.Counter) float64 {
	var metric dto.Metric
	err := counter.Write(&metric)
	if err != nil {
		return 0.0
	}
	return metric.Counter.GetValue()
}

func BenchmarkEscapeMetricName(b *testing.B) {
	scenarios := []string{
		"clean",
		"0starts_with_digit",
		"with_underscore",
		"with.dot",
		"with😱emoji",
		"with.*.multiple",
		"test.web-server.foo.bar",
		"",
	}

	for _, s := range scenarios {
		b.Run(s, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				escapeMetricName(s)
			}
		})
	}
}

func BenchmarkParseDogStatsDTagsToLabels(b *testing.B) {
	scenarios := map[string]string{
		"1 tag w/hash":         "#test:tag",
		"1 tag w/o hash":       "test:tag",
		"2 tags, mixed hashes": "tag1:test,#tag2:test",
		"3 long tags":          "tag1:reallylongtagthisisreallylong,tag2:anotherreallylongtag,tag3:thisisyetanotherextraordinarilylongtag",
		"a-z tags":             "a:0,b:1,c:2,d:3,e:4,f:5,g:6,h:7,i:8,j:9,k:0,l:1,m:2,n:3,o:4,p:5,q:6,r:7,s:8,t:9,u:0,v:1,w:2,x:3,y:4,z:5",
	}

	for name, tags := range scenarios {
		b.Run(name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				parseDogStatsDTagsToLabels(tags)
			}
		})
	}
}

func BenchmarkHashNameAndLabels(b *testing.B) {
	scenarios := []struct {
		name   string
		metric string
		labels map[string]string
	}{
		{
			name:   "no labels",
			labels: map[string]string{},
		}, {
			name: "one label",
			labels: map[string]string{
				"label": "value",
			},
		}, {
			name: "many labels",
			labels: map[string]string{
				"label0": "value",
				"label1": "value",
				"label2": "value",
				"label3": "value",
				"label4": "value",
				"label5": "value",
				"label6": "value",
				"label7": "value",
				"label8": "value",
				"label9": "value",
			},
		},
	}

	r := newRegistry(nil)
	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				r.hashLabels(s.labels)
			}
		})
	}
}
