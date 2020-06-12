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
	"reflect"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/event"
	"github.com/prometheus/statsd_exporter/pkg/exporter"
	"github.com/prometheus/statsd_exporter/pkg/listener"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

func TestHandlePacket(t *testing.T) {
	scenarios := []struct {
		name string
		in   string
		out  event.Events
	}{
		{
			name: "empty",
		}, {
			name: "simple counter",
			in:   "foo:2|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      2,
					CLabels:     map[string]string{},
				},
			},
		}, {
			name: "simple gauge",
			in:   "foo:3|g",
			out: event.Events{
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      3,
					GLabels:     map[string]string{},
				},
			},
		}, {
			name: "gauge with sampling",
			in:   "foo:3|g|@0.2",
			out: event.Events{
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      3,
					GLabels:     map[string]string{},
				},
			},
		}, {
			name: "gauge decrement",
			in:   "foo:-10|g",
			out: event.Events{
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      -10,
					GRelative:   true,
					GLabels:     map[string]string{},
				},
			},
		}, {
			name: "simple timer",
			in:   "foo:200|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.2,
					OLabels:     map[string]string{},
				},
			},
		}, {
			name: "simple histogram",
			in:   "foo:200|h",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		}, {
			name: "simple distribution",
			in:   "foo:200|d",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		}, {
			name: "distribution with sampling",
			in:   "foo:0.01|d|@0.2|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "librato tag extension",
			in:   "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "librato tag extension with tag keys unsupported by prometheus",
			in:   "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		}, {
			name: "influxdb tag extension",
			in:   "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "influxdb tag extension with tag keys unsupported by prometheus",
			in:   "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		}, {
			name: "datadog tag extension",
			in:   "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "datadog tag extension with # in all keys (as sent by datadog php client)",
			in:   "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "datadog tag extension with tag keys unsupported by prometheus",
			in:   "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		}, {
			name: "datadog tag extension with valueless tags: ignored",
			in:   "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		}, {
			name: "datadog tag extension with valueless tags (edge case)",
			in:   "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		}, {
			name: "datadog tag extension with empty tags (edge case)",
			in:   "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		}, {
			name: "datadog tag extension with sampling",
			in:   "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "librato/dogstatsd mixed tag styles without sampling",
			in:   "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out:  event.Events{},
		}, {
			name: "influxdb/dogstatsd mixed tag styles without sampling",
			in:   "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out:  event.Events{},
		}, {
			name: "mixed tag styles with sampling",
			in:   "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out:  event.Events{},
		}, {
			name: "histogram with sampling",
			in:   "foo:0.01|h|@0.2|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		}, {
			name: "datadog tag extension with multiple colons",
			in:   "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "foo:bar"},
				},
			},
		}, {
			name: "datadog tag extension with invalid utf8 tag values",
			in:   "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		}, {
			name: "datadog tag extension with both valid and invalid utf8 tag values",
			in:   "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		}, {
			name: "multiple metrics with invalid datadog utf8 tag values",
			in:   "foo:200|c|#tag:value\nfoo:300|c|#tag:\xc3\x28invalid",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      200,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		}, {
			name: "combined multiline metrics",
			in:   "foo:200|ms:300|ms:5|c|@0.1:6|g\nbar:1|c:5|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      .200,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      .300,
					OLabels:     map[string]string{},
				},
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      50,
					CLabels:     map[string]string{},
				},
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      6,
					GLabels:     map[string]string{},
				},
				&event.CounterEvent{
					CMetricName: "bar",
					CValue:      1,
					CLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "bar",
					OValue:      .005,
					OLabels:     map[string]string{},
				},
			},
		}, {
			name: "timings with sampling factor",
			in:   "foo.timing:0.5|ms|@0.1",
			out: event.Events{
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
			},
		}, {
			name: "bad line",
			in:   "foo",
		}, {
			name: "bad component",
			in:   "foo:1",
		}, {
			name: "bad value",
			in:   "foo:1o|c",
		}, {
			name: "illegal sampling factor",
			in:   "foo:1|c|@bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1,
					CLabels:     map[string]string{},
				},
			},
		}, {
			name: "zero sampling factor",
			in:   "foo:2|c|@0",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      2,
					CLabels:     map[string]string{},
				},
			},
		}, {
			name: "illegal stat type",
			in:   "foo:2|t",
		},
		{
			name: "empty metric name",
			in:   ":100|ms",
		},
		{
			name: "empty component",
			in:   "foo:1|c|",
		},
		{
			name: "invalid utf8",
			in:   "invalid\xc3\x28utf8:1|c",
		},
		{
			name: "some invalid utf8",
			in:   "valid_utf8:1|c\ninvalid\xc3\x28utf8:1|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "valid_utf8",
					CValue:      1,
					CLabels:     map[string]string{},
				},
			},
		}, {
			name: "ms timer with conversion to seconds",
			in:   "foo:200|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.2,
					OLabels:     map[string]string{},
				},
			},
		}, {
			name: "histogram with no unit conversion",
			in:   "foo:200|h",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		}, {
			name: "distribution with no unit conversion",
			in:   "foo:200|d",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		},
	}

	for k, l := range []statsDPacketHandler{&listener.StatsDUDPListener{
		Conn:            nil,
		EventHandler:    nil,
		Logger:          log.NewNopLogger(),
		UDPPackets:      udpPackets,
		LinesReceived:   linesReceived,
		EventsFlushed:   eventsFlushed,
		SampleErrors:    *sampleErrors,
		SamplesReceived: samplesReceived,
		TagErrors:       tagErrors,
		TagsReceived:    tagsReceived,
	}, &mockStatsDTCPListener{listener.StatsDTCPListener{
		Conn:            nil,
		EventHandler:    nil,
		Logger:          log.NewNopLogger(),
		LinesReceived:   linesReceived,
		EventsFlushed:   eventsFlushed,
		SampleErrors:    *sampleErrors,
		SamplesReceived: samplesReceived,
		TagErrors:       tagErrors,
		TagsReceived:    tagsReceived,
		TCPConnections:  tcpConnections,
		TCPErrors:       tcpErrors,
		TCPLineTooLong:  tcpLineTooLong,
	}, log.NewNopLogger()}} {
		events := make(chan event.Events, 32)
		l.SetEventHandler(&event.UnbufferedEventHandler{C: events})
		for i, scenario := range scenarios {
			l.HandlePacket([]byte(scenario.in))

			le := len(events)
			// Flatten actual events.
			actual := event.Events{}
			for i := 0; i < le; i++ {
				actual = append(actual, <-events...)
			}

			if len(actual) != len(scenario.out) {
				t.Fatalf("%d.%d. Expected %d events, got %d in scenario '%s'", k, i, len(scenario.out), len(actual), scenario.name)
			}

			for j, expected := range scenario.out {
				if !reflect.DeepEqual(&expected, &actual[j]) {
					t.Fatalf("%d.%d.%d. Expected %#v, got %#v in scenario '%s'", k, i, j, expected, actual[j], scenario.name)
				}
			}
		}
	}
}

type statsDPacketHandler interface {
	HandlePacket(packet []byte)
	SetEventHandler(eh event.EventHandler)
}

type mockStatsDTCPListener struct {
	listener.StatsDTCPListener
	log.Logger
}

func (ml *mockStatsDTCPListener) HandlePacket(packet []byte) {
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
	ml.HandleConn(sc)
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
	events := make(chan event.Events)
	defer close(events)
	go func() {
		ex := exporter.NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
		ex.Listen(events)
	}()

	ev := event.Events{
		// event with default ttl = 1s
		&event.GaugeEvent{
			GMetricName: "foobar",
			GValue:      200,
		},
		// event with ttl = 2s from a mapping
		&event.ObserverEvent{
			OMetricName: "bazqux.main",
			OValue:      42,
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
	events <- event.Events{}

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
	events <- event.Events{}

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
	events <- event.Events{}

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
