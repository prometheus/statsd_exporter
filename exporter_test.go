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

	ex := NewExporter(&mapper.MetricMapper{})
	ex.Listen(events)
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
	err := testMapper.InitFromYAMLString(config)
	if err != nil {
		t.Fatalf("Config load error: %s %s", config, err)
	}

	ex := NewExporter(testMapper)
	ex.Listen(events)
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

	go func() {
		for _, l := range []statsDPacketHandler{&StatsDUDPListener{}, &mockStatsDTCPListener{}} {
			l.handlePacket([]byte("bar:200|c|#tag:value\nbar:200|c|#tag:\xc3\x28invalid"), events)
		}
		close(events)
	}()

	ex := NewExporter(&mapper.MetricMapper{})
	ex.Listen(events)
}

func TestHistogramUnits(t *testing.T) {
	// Start exporter with a synchronous channel
	events := make(chan Events)
	go func() {
		ex := NewExporter(&mapper.MetricMapper{})
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

type statsDPacketHandler interface {
	handlePacket(packet []byte, e chan<- Events)
}

type mockStatsDTCPListener struct {
	StatsDTCPListener
}

func (ml *mockStatsDTCPListener) handlePacket(packet []byte, e chan<- Events) {
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
	ml.handleConn(sc, e)
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
		"": "",
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
	err := testMapper.InitFromYAMLString(config)
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
	labelsHash := hashNameAndLabels(name, labels)
	for _, m := range metricFamily.Metric {
		h := hashNameAndLabels(name, labelPairsAsLabels(m.GetLabel()))
		if h == labelsHash {
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
