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
	"testing"

	"github.com/go-kit/kit/log"

	"github.com/prometheus/statsd_exporter/pkg/event"
	"github.com/prometheus/statsd_exporter/pkg/exporter"
	"github.com/prometheus/statsd_exporter/pkg/listener"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

func benchmarkUDPListener(times int, b *testing.B) {
	input := []string{
		"foo1:2|c",
		"foo2:3|g",
		"foo3:200|ms",
		"foo4:100|c|#tag1:bar,tag2:baz",
		"foo5:100|c|#tag1:bar,#tag2:baz",
		"foo6:100|c|#09digits:0,tag.with.dots:1",
		"foo10:100|c|@0.1|#tag1:bar,#tag2:baz",
		"foo11:100|c|@0.1|#tag1:foo:bar",
		"foo.[foo=bar,dim=val]test:1|g",
		"foo15:200|ms:300|ms:5|c|@0.1:6|g\nfoo15a:1|c:5|ms",
		"some_very_useful_metrics_with_quite_a_log_name:13|c",
	}
	bytesInput := make([]string, len(input)*times)
	logger := log.NewNopLogger()
	for run := 0; run < times; run++ {
		for i := 0; i < len(input); i++ {
			bytesInput[run*len(input)+i] = fmt.Sprintf("run%d%s", run, input[i])
		}
	}

	// reset benchmark timer to not measure startup costs
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		// pause benchmark timer for creating the chan and listener
		b.StopTimer()

		// there are more events than input lines, need bigger buffer
		events := make(chan event.Events, len(bytesInput)*times*2)

		l := listener.StatsDUDPListener{
			EventHandler:    &event.UnbufferedEventHandler{C: events},
			Logger:          logger,
			UDPPackets:      udpPackets,
			LinesReceived:   linesReceived,
			SamplesReceived: samplesReceived,
			TagsReceived:    tagsReceived,
		}

		// resume benchmark timer
		b.StartTimer()

		for i := 0; i < times; i++ {
			for _, line := range bytesInput {
				l.HandlePacket([]byte(line))
			}
		}
	}
}

func BenchmarkUDPListener1(b *testing.B) {
	benchmarkUDPListener(1, b)
}
func BenchmarkUDPListener5(b *testing.B) {
	benchmarkUDPListener(5, b)
}
func BenchmarkUDPListener50(b *testing.B) {
	benchmarkUDPListener(50, b)
}

func BenchmarkExporterListener(b *testing.B) {
	events := event.Events{
		&event.CounterEvent{ // simple counter
			CMetricName: "counter",
			CValue:      2,
		},
		&event.GaugeEvent{ // simple gauge
			GMetricName: "gauge",
			GValue:      10,
		},
		&event.ObserverEvent{ // simple timer
			OMetricName: "timer",
			OValue:      200,
		},
		&event.ObserverEvent{ // simple histogram
			OMetricName: "histogram.test",
			OValue:      200,
		},
		&event.CounterEvent{ // simple_tags
			CMetricName: "simple_tags",
			CValue:      100,
			CLabels: map[string]string{
				"alpha": "bar",
				"bravo": "baz",
			},
		},
		&event.CounterEvent{ // slightly different tags
			CMetricName: "simple_tags",
			CValue:      100,
			CLabels: map[string]string{
				"alpha":   "bar",
				"charlie": "baz",
			},
		},
		&event.CounterEvent{ // and even more different tags
			CMetricName: "simple_tags",
			CValue:      100,
			CLabels: map[string]string{
				"alpha": "bar",
				"bravo": "baz",
				"golf":  "looooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong",
			},
		},
		&event.CounterEvent{ // datadog tag extension with complex tags
			CMetricName: "foo",
			CValue:      100,
			CLabels: map[string]string{
				"action":                "test",
				"application":           "testapp",
				"application_component": "testcomp",
				"application_role":      "test_role",
				"category":              "category",
				"controller":            "controller",
				"deployed_to":           "production",
				"kube_deployment":       "deploy",
				"kube_namespace":        "kube-production",
				"method":                "get",
				"version":               "5.2.8374",
				"status":                "200",
				"status_range":          "2xx",
			},
		},
	}
	config := `
mappings:
- match: histogram.test
  timer_type: histogram
  name: "histogram_test"
`

	testMapper := &mapper.MetricMapper{}
	err := testMapper.InitFromYAMLString(config, 0)
	if err != nil {
		b.Fatalf("Config load error: %s %s", config, err)
	}

	ex := exporter.NewExporter(testMapper, log.NewNopLogger(), eventsActions, eventsUnmapped, errorEventStats, eventStats, conflictingEventStats, metricsCount)

	// reset benchmark timer to not measure startup costs
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// pause benchmark timer for creating the chan
		b.StopTimer()

		ec := make(chan event.Events, 1000)

		// resume benchmark timer
		b.StartTimer()

		go func() {
			for i := 0; i < 1000; i++ {
				ec <- events
			}
			close(ec)
		}()

		ex.Listen(ec)
	}
}
