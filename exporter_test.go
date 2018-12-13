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

	events := make(chan Events, 1)
	c := Events{
		&CounterEvent{
			metricName: "foo",
			value:      -1,
		},
	}
	events <- c
	ex := NewExporter(&mapper.MetricMapper{})

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()

	ex.Listen(events)
}

// TestInvalidUtf8InDatadogTagValue validates robustness of exporter listener
// against datadog tags with invalid tag values.
// It sends the same tags first with a valid value, then with an invalid one.
// The exporter should not panic, but drop the invalid event
func TestInvalidUtf8InDatadogTagValue(t *testing.T) {
	ex := NewExporter(&mapper.MetricMapper{})
	for _, l := range []statsDPacketHandler{&StatsDUDPListener{}, &mockStatsDTCPListener{}} {
		events := make(chan Events, 2)

		l.handlePacket([]byte("bar:200|c|#tag:value\nbar:200|c|#tag:\xc3\x28invalid"), events)

		// Close channel to signify we are done with the listener after a short period.
		go func() {
			time.Sleep(time.Millisecond * 100)
			close(events)
		}()

		ex.Listen(events)
	}
}

func TestHistogramUnits(t *testing.T) {
	events := make(chan Events, 1)
	name := "foo"
	c := Events{
		&TimerEvent{
			metricName: name,
			value:      300,
		},
	}
	events <- c
	ex := NewExporter(&mapper.MetricMapper{})
	ex.mapper.Defaults.TimerType = mapper.TimerTypeHistogram

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()

	ex.Listen(events)

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
	}
	value := getFloat64(metrics, name, prometheus.Labels{})
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
	config := `
defaults:
  ttl: 1s
mappings:
- match: bazqux.*
  name: bazqux
  labels:
    first: baz
    second: qux
    third: $1
  ttl: 2s
`

	bazquxLabels := prometheus.Labels{
		"third":  "main",
		"first":  "baz",
		"second": "qux",
	}

	testMapper := &mapper.MetricMapper{}
	err := testMapper.InitFromYAMLString(config)
	if err != nil {
		t.Fatalf("Config load error: %s %s", config, err)
	}

	ex := NewExporter(testMapper)
	for _, l := range []statsDPacketHandler{&StatsDUDPListener{}, &mockStatsDTCPListener{}} {
		events := make(chan Events, 2)
		fatal := make(chan error, 1) // t.Fatal must not be called in goroutines (SA2002)
		stop := make(chan bool, 1)

		l.handlePacket([]byte("foobar:200|g"), events)
		l.handlePacket([]byte("bazqux.main:42|ms"), events)

		// Close channel to signify we are done with the listener after a short period.
		go func() {
			defer close(events)

			time.Sleep(time.Millisecond * 100)

			var metrics []*dto.MetricFamily
			var foobarValue *float64
			var bazquxValue *float64

			// Wait to gather both metrics
			var tries = 7
			for {
				metrics, err = prometheus.DefaultGatherer.Gather()

				foobarValue = getFloat64(metrics, "foobar", prometheus.Labels{})
				bazquxValue = getFloat64(metrics, "bazqux", bazquxLabels)
				if foobarValue != nil && bazquxValue != nil {
					break
				}

				tries--
				if tries == 0 {
					fatal <- fmt.Errorf("Gauge `foobar` and Summary `bazqux` should be gathered")
					return
				}
				time.Sleep(time.Millisecond * 100)
			}

			// Check values
			if *foobarValue != 200 {
				fatal <- fmt.Errorf("Gauge `foobar` observation %f is not expected. Should be 200", *foobarValue)
				return
			}
			if *bazquxValue != 42 {
				fatal <- fmt.Errorf("Summary `bazqux` observation %f is not expected. Should be 42", *bazquxValue)
				return
			}

			// Wait for expiration of foobar
			tries = 20 // 20*100 = 2s
			for {
				time.Sleep(time.Millisecond * 100)
				metrics, err = prometheus.DefaultGatherer.Gather()

				foobarValue = getFloat64(metrics, "foobar", prometheus.Labels{})
				bazquxValue = getFloat64(metrics, "bazqux", bazquxLabels)
				if foobarValue == nil {
					break
				}

				tries--
				if tries == 0 {
					fatal <- fmt.Errorf("Gauge `foobar` should be expired")
					return
				}
			}

			if *bazquxValue != 42 {
				fatal <- fmt.Errorf("Summary `bazqux` observation %f is not expected. Should be 42", *bazquxValue)
				return
			}

			// Wait for expiration of bazqux
			tries = 20 // 20*100 = 2s
			for {
				time.Sleep(time.Millisecond * 100)
				metrics, err = prometheus.DefaultGatherer.Gather()

				foobarValue = getFloat64(metrics, "foobar", prometheus.Labels{})
				bazquxValue = getFloat64(metrics, "bazqux", bazquxLabels)
				if bazquxValue == nil {
					break
				}
				if foobarValue != nil {
					fatal <- fmt.Errorf("Gauge `foobar` should not be gathered after expiration")
					return
				}

				tries--
				if tries == 0 {
					fatal <- fmt.Errorf("Summary `bazqux` should be expired")
					return
				}
			}
		}()

		go func() {
			ex.Listen(events)
			stop <- true
		}()

		for {
			select {
			case err := <-fatal:
				t.Fatalf("%v", err)
			case <-stop:
				return
			}
		}

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
