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

	"github.com/prometheus/client_golang/prometheus"
)

func benchmarkExporter(times int, b *testing.B) {
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

func BenchmarkExporter1(b *testing.B) {
	benchmarkExporter(1, b)
}
func BenchmarkExporter5(b *testing.B) {
	benchmarkExporter(5, b)
}
func BenchmarkExporter50(b *testing.B) {
	benchmarkExporter(50, b)
}

type metricGenerator struct {
	metrics int
	labels  int
}

func (gen metricGenerator) Generate(out chan Events) {
	labels := []map[string]string{}
	for l := 0; l < gen.labels; l++ {
		labels = append(labels, map[string]string{
			"the_label": fmt.Sprintf("%s", l),
		})
	}

	for m := 0; m < gen.metrics; m++ {
		name := fmt.Sprintf("metric%s", m)
		for _, l := range labels {
			e := &GaugeEvent{
				metricName: name,
				value:      float64(m),
				relative:   false,
				labels:     l,
			}
			out <- Events{e}
		}
	}
	close(out)
}

var cases = []metricGenerator{
	metricGenerator{1, 0},
	metricGenerator{1, 1},
	metricGenerator{10, 0},
	metricGenerator{1, 10},
	metricGenerator{10, 10},
	metricGenerator{100, 0},
	metricGenerator{1, 100},
	metricGenerator{100, 100},
	metricGenerator{1000, 10},
	metricGenerator{10000, 10},
	metricGenerator{100000, 10},
	metricGenerator{1000000, 1},
	metricGenerator{10, 100},
	metricGenerator{10, 1000},
	metricGenerator{10, 10000},
	metricGenerator{10, 100000},
	metricGenerator{1, 1000000},
}

var dummy Events

func BenchmarkGenerator(b *testing.B) {
	for _, c := range cases {
		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()

				events := make(chan Events, 1000)
				go func() {
					for e := range events {
						dummy = e
					}
				}()

				b.StartTimer()
				c.Generate(events)
			}
		})
	}
}

func BenchmarkGather(b *testing.B) {
	mapper := &metricMapper{}
	mapper.initFromYAMLString("")

	for _, c := range cases {
		// reset the global Prometheus registry
		registry := prometheus.NewRegistry()
		prometheus.DefaultRegisterer = registry
		prometheus.DefaultGatherer = registry

		// Make a fresh exporter
		exporter := NewExporter(mapper)

		// And feed it some metrics
		events := make(chan Events, 1000)
		go c.Generate(events)
		exporter.Listen(events)

		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, _ = prometheus.DefaultGatherer.Gather()
			}
		})
	}
}
