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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/event"
	"github.com/prometheus/statsd_exporter/pkg/line"
	"github.com/prometheus/statsd_exporter/pkg/listener"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
	"github.com/prometheus/statsd_exporter/pkg/registry"
)

var (
	eventStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_total",
			Help: "The total number of StatsD events seen.",
		},
		[]string{"type"},
	)
	eventsFlushed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_event_queue_flushed_total",
			Help: "Number of times events were flushed to exporter",
		},
	)
	eventsUnmapped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_unmapped_total",
			Help: "The total number of StatsD events no mapping was found for.",
		})
	udpPackets = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_udp_packets_total",
			Help: "The total number of StatsD packets received over UDP.",
		},
	)
	tcpConnections = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tcp_connections_total",
			Help: "The total number of TCP connections handled.",
		},
	)
	tcpErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tcp_connection_errors_total",
			Help: "The number of errors encountered reading from TCP.",
		},
	)
	tcpLineTooLong = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tcp_too_long_lines_total",
			Help: "The number of lines discarded due to being too long.",
		},
	)
	unixgramPackets = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_unixgram_packets_total",
			Help: "The total number of StatsD packets received over Unixgram.",
		},
	)
	linesReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_lines_total",
			Help: "The total number of StatsD lines received.",
		},
	)
	samplesReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_samples_total",
			Help: "The total number of StatsD samples received.",
		},
	)
	sampleErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_sample_errors_total",
			Help: "The total number of errors parsing StatsD samples.",
		},
		[]string{"reason"},
	)
	tagsReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tags_total",
			Help: "The total number of DogStatsD tags processed.",
		},
	)
	tagErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tag_errors_total",
			Help: "The number of errors parsing DogStatsD tags.",
		},
	)
	configLoads = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_config_reloads_total",
			Help: "The number of configuration reloads.",
		},
		[]string{"outcome"},
	)
	mappingsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "statsd_exporter_loaded_mappings",
		Help: "The current number of configured metric mappings.",
	})
	conflictingEventStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_conflict_total",
			Help: "The total number of StatsD events with conflicting names.",
		},
		[]string{"type"},
	)
	errorEventStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_error_total",
			Help: "The total number of StatsD events discarded due to errors.",
		},
		[]string{"reason"},
	)
	eventsActions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_actions_total",
			Help: "The total number of StatsD events by action.",
		},
		[]string{"action"},
	)
	metricsCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "statsd_exporter_metrics_total",
			Help: "The total number of metrics.",
		},
		[]string{"type"},
	)
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

	events := make(chan event.Events)
	go func() {
		c := event.Events{
			&event.CounterEvent{
				CMetricName: "foo",
				CValue:      -1,
			},
		}
		events <- c
		close(events)
	}()

	errorCounter := errorEventStats.WithLabelValues("illegal_negative_counter")
	prev := getTelemetryCounterValue(errorCounter)

	testMapper := mapper.MetricMapper{}
	testMapper.InitCache(0)

	ex := NewExporter(&testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
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

	events := make(chan event.Events)
	go func() {
		c := event.Events{
			&event.CounterEvent{
				CMetricName: "counter_test",
				CValue:      1,
				CLabels:     firstLabelSet,
			},
			&event.CounterEvent{
				CMetricName: "counter_test",
				CValue:      1,
				CLabels:     secondLabelSet,
			},
			&event.GaugeEvent{
				GMetricName: "gauge_test",
				GValue:      1,
				GLabels:     firstLabelSet,
			},
			&event.GaugeEvent{
				GMetricName: "gauge_test",
				GValue:      1,
				GLabels:     secondLabelSet,
			},
			&event.ObserverEvent{
				OMetricName: "histogram.test",
				OValue:      1,
				OLabels:     firstLabelSet,
			},
			&event.ObserverEvent{
				OMetricName: "histogram.test",
				OValue:      1,
				OLabels:     secondLabelSet,
			},
			&event.ObserverEvent{
				OMetricName: "summary_test",
				OValue:      1,
				OLabels:     firstLabelSet,
			},
			&event.ObserverEvent{
				OMetricName: "summary_test",
				OValue:      1,
				OLabels:     secondLabelSet,
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

	ex := NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
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

	events := make(chan event.Events)
	go func() {
		c := event.Events{
			&event.CounterEvent{
				CMetricName: "counter.test.200",
				CValue:      1,
				CLabels:     make(map[string]string),
			},
			&event.CounterEvent{
				CMetricName: "counter.test.300",
				CValue:      1,
				CLabels:     make(map[string]string),
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

	ex := NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
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
		in       event.Events
	}{
		{
			name:     "counter vs gauge",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "cvg_test",
					CValue:      1,
				},
				&event.GaugeEvent{
					GMetricName: "cvg_test",
					GValue:      2,
				},
			},
		},
		{
			name:     "counter vs gauge with different labels",
			expected: []float64{1, 2},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "cvgl_test",
					CValue:      1,
					CLabels:     map[string]string{"tag": "1"},
				},
				&event.CounterEvent{
					CMetricName: "cvgl_test",
					CValue:      2,
					CLabels:     map[string]string{"tag": "2"},
				},
				&event.GaugeEvent{
					GMetricName: "cvgl_test",
					GValue:      3,
					GLabels:     map[string]string{"tag": "1"},
				},
			},
		},
		{
			name:     "counter vs gauge with same labels",
			expected: []float64{3},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "cvgsl_test",
					CValue:      1,
					CLabels:     map[string]string{"tag": "1"},
				},
				&event.CounterEvent{
					CMetricName: "cvgsl_test",
					CValue:      2,
					CLabels:     map[string]string{"tag": "1"},
				},
				&event.GaugeEvent{
					GMetricName: "cvgsl_test",
					GValue:      3,
					GLabels:     map[string]string{"tag": "1"},
				},
			},
		},
		{
			name:     "gauge vs counter",
			expected: []float64{2},
			in: event.Events{
				&event.GaugeEvent{
					GMetricName: "gvc_test",
					GValue:      2,
				},
				&event.CounterEvent{
					CMetricName: "gvc_test",
					CValue:      1,
				},
			},
		},
		{
			name:     "counter vs histogram",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "histogram_test1",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "histogram.test1",
					OValue:      2,
				},
			},
		},
		{
			name:     "counter vs histogram sum",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "histogram_test1_sum",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "histogram.test1",
					OValue:      2,
				},
			},
		},
		{
			name:     "counter vs histogram count",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "histogram_test2_count",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "histogram.test2",
					OValue:      2,
				},
			},
		},
		{
			name:     "counter vs histogram bucket",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "histogram_test3_bucket",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "histogram.test3",
					OValue:      2,
				},
			},
		},
		{
			name:     "counter vs summary quantile",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "cvsq_test",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "cvsq_test",
					OValue:      2,
				},
			},
		},
		{
			name:     "counter vs summary count",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "cvsc_count",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "cvsc",
					OValue:      2,
				},
			},
		},
		{
			name:     "counter vs summary sum",
			expected: []float64{1},
			in: event.Events{
				&event.CounterEvent{
					CMetricName: "cvss_sum",
					CValue:      1,
				},
				&event.ObserverEvent{
					OMetricName: "cvss",
					OValue:      2,
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

			events := make(chan event.Events)
			go func() {
				events <- s.in
				close(events)
			}()
			ex := NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
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
	events := make(chan event.Events)
	go func() {
		c := event.Events{
			&event.CounterEvent{
				CMetricName: "foo_bar",
				CValue:      1,
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

	ex := NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
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

	events := make(chan event.Events)
	ueh := &event.UnbufferedEventHandler{C: events}

	go func() {
		for _, l := range []statsDPacketHandler{&listener.StatsDUDPListener{
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
			l.SetEventHandler(ueh)
			l.HandlePacket([]byte("bar:200|c|#tag:value\nbar:200|c|#tag:\xc3\x28invalid"))
		}
		close(events)
	}()

	testMapper := mapper.MetricMapper{}
	testMapper.InitCache(0)

	ex := NewExporter(&testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
	ex.Listen(events)
}

// In the case of someone starting the statsd exporter with no mapping file specified
// which is valid, we want to make sure that the default quantile metrics are generated
// as well as the sum/count metrics
func TestSummaryWithQuantilesEmptyMapping(t *testing.T) {
	// Start exporter with a synchronous channel
	events := make(chan event.Events)
	go func() {
		testMapper := mapper.MetricMapper{}
		testMapper.InitCache(0)

		ex := NewExporter(&testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
		ex.Listen(events)
	}()

	name := "default_foo"
	c := event.Events{
		&event.ObserverEvent{
			OMetricName: name,
			OValue:      300,
		},
	}
	events <- c
	events <- event.Events{}
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
	events := make(chan event.Events)
	go func() {
		testMapper := mapper.MetricMapper{}
		testMapper.InitCache(0)
		ex := NewExporter(&testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
		ex.Mapper.Defaults.TimerType = mapper.TimerTypeHistogram
		ex.Listen(events)
	}()

	// Synchronously send a statsd event to wait for handleEvent execution.
	// Then close events channel to stop a listener.
	name := "foo"
	c := event.Events{
		&event.ObserverEvent{
			OMetricName: name,
			OValue:      .300,
		},
	}
	events <- c
	events <- event.Events{}
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
	if *value != .300 {
		t.Fatalf("Received unexpected value for histogram observation %f != .300", *value)
	}
}
func TestCounterIncrement(t *testing.T) {
	// Start exporter with a synchronous channel
	events := make(chan event.Events)
	go func() {
		testMapper := mapper.MetricMapper{}
		testMapper.InitCache(0)
		ex := NewExporter(&testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
		ex.Listen(events)
	}()

	// Synchronously send a statsd event to wait for handleEvent execution.
	// Then close events channel to stop a listener.
	name := "foo_counter"
	labels := map[string]string{
		"foo": "bar",
	}
	c := event.Events{
		&event.CounterEvent{
			CMetricName: name,
			CValue:      1,
			CLabels:     labels,
		},
		&event.CounterEvent{
			CMetricName: name,
			CValue:      1,
			CLabels:     labels,
		},
	}
	events <- c
	// Push empty event so that we block until the first event is consumed.
	events <- event.Events{}
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
		ex := NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)
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

func TestHashLabelNames(t *testing.T) {
	r := registry.NewRegistry(nil)
	// Validate value hash changes and name has doesn't when just the value changes.
	hash1, _ := r.HashLabels(map[string]string{
		"label": "value1",
	})
	hash2, _ := r.HashLabels(map[string]string{
		"label": "value2",
	})
	if hash1.Names != hash2.Names {
		t.Fatal("Hash of label names should match, but doesn't")
	}
	if hash1.Values == hash2.Values {
		t.Fatal("Hash of label names shouldn't match, but do")
	}

	// Validate value and name hashes change when the name changes.
	hash1, _ = r.HashLabels(map[string]string{
		"label1": "value",
	})
	hash2, _ = r.HashLabels(map[string]string{
		"label2": "value",
	})
	if hash1.Names == hash2.Names {
		t.Fatal("Hash of label names shouldn't match, but do")
	}
	if hash1.Values == hash2.Values {
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

func BenchmarkParseDogStatsDTags(b *testing.B) {
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
				labels := map[string]string{}
				line.ParseDogStatsDTags(tags, labels, tagErrors, log.NewNopLogger())
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

	r := registry.NewRegistry(nil)
	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				r.HashLabels(s.labels)
			}
		})
	}
}
