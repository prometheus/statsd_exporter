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
		"foo15:200|ms:300|ms:5|c|@0.1:6|g\nfoo15a:1|c:5|ms",
		"some_very_useful_metrics_with_quite_a_log_name:13|c",
	}
	bytesInput := make([]string, len(input)*times)
	for run := 0; run < times; run++ {
		for i := 0; i < len(input); i++ {
			bytesInput[run*len(input)+i] = fmt.Sprintf("run%d%s", run, input[i])
		}
	}
	for n := 0; n < b.N; n++ {
		l := StatsDUDPListener{}
		// there are more events than input lines, need bigger buffer
		events := make(chan Events, len(bytesInput)*times*2)

		for i := 0; i < times; i++ {
			for _, line := range bytesInput {
				l.handlePacket([]byte(line), events)
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
	events := Events{
		&CounterEvent{ // simple counter
			metricName: "counter",
			value:      2,
		},
		&GaugeEvent{ // simple gauge
			metricName: "gauge",
			value:      10,
		},
		&TimerEvent{ // simple timer
			metricName: "timer",
			value:      200,
		},
		&TimerEvent{ // simple histogram
			metricName: "histogram.test",
			value:      200,
		},
		&CounterEvent{ // simple_tags
			metricName: "simple_tags",
			value:      100,
			labels: map[string]string{
				"alpha": "bar",
				"bravo": "baz",
			},
		},
		&CounterEvent{ // slightly different tags
			metricName: "simple_tags",
			value:      100,
			labels: map[string]string{
				"alpha":   "bar",
				"charlie": "baz",
			},
		},
		&CounterEvent{ // and even more different tags
			metricName: "simple_tags",
			value:      100,
			labels: map[string]string{
				"alpha": "bar",
				"bravo": "baz",
				"golf":  "looooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooong",
			},
		},
		&CounterEvent{ // datadog tag extension with complex tags
			metricName: "foo",
			value:      100,
			labels: map[string]string{
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

	ex := NewExporter(testMapper)
	for i := 0; i < b.N; i++ {
		ec := make(chan Events, 1000)
		go func() {
			for i := 0; i < 1000; i++ {
				ec <- events
			}
			close(ec)
		}()

		ex.Listen(ec)
	}
}
