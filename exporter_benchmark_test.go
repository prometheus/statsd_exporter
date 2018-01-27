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
	"runtime"
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
			"the_label": fmt.Sprintf("%d", l),
		})
	}

	for m := 0; m < gen.metrics; m++ {
		name := fmt.Sprintf("metric%d", m)
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
	metricGenerator{100000, 1},
	metricGenerator{10000, 10},
	metricGenerator{10, 10000},
	metricGenerator{1, 100000},
}

func BenchmarkGenerator(b *testing.B) {
	for _, c := range cases {
		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()

				events := make(chan Events, 1000)
				go func() {
					for range events {
					}
				}()

				b.StartTimer()
				c.Generate(events)
			}
		})
	}
}

func (gen metricGenerator) observeGauge(exporter *Exporter) {
	metricNames := make([]string, 0, gen.metrics)
	for m := 0; m < gen.metrics; m++ {
		metricNames = append(metricNames, fmt.Sprintf("metric%d", m))
	}
	labels := make([]map[string]string, 0, gen.labels)
	for l := 0; l < gen.labels; l++ {
		labels = append(labels, map[string]string{"the_label": fmt.Sprintf("label%d", l)})
	}

	for _, mn := range metricNames {
		for _, lv := range labels {
			gauge, _ := exporter.Gauges.Get(mn, lv, "help")
			gauge.Set(float64(1.0))
		}
	}
}

func BenchmarkGatherGauge(b *testing.B) {
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
		c.observeGauge(exporter)
		runtime.GC()
		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, _ = prometheus.DefaultGatherer.Gather()
			}
		})
	}
}

func (gen metricGenerator) observeCounter(exporter *Exporter) {
	metricNames := make([]string, 0, gen.metrics)
	for m := 0; m < gen.metrics; m++ {
		metricNames = append(metricNames, fmt.Sprintf("metric%d", m))
	}
	labels := make([]map[string]string, 0, gen.labels)
	for l := 0; l < gen.labels; l++ {
		labels = append(labels, map[string]string{"the_label": fmt.Sprintf("label%d", l)})
	}

	for _, mn := range metricNames {
		for _, lv := range labels {
			counter, _ := exporter.Counters.Get(mn, lv, "help")
			counter.Add(float64(1.0))
		}
	}
}

func BenchmarkGatherCounter(b *testing.B) {
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
		c.observeCounter(exporter)
		runtime.GC()
		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, _ = prometheus.DefaultGatherer.Gather()
			}
		})
	}
}

func (gen metricGenerator) observeSummary(exporter *Exporter) {
	metricNames := make([]string, 0, gen.metrics)
	for m := 0; m < gen.metrics; m++ {
		metricNames = append(metricNames, fmt.Sprintf("metric%d", m))
	}
	labels := make([]map[string]string, 0, gen.labels)
	for l := 0; l < gen.labels; l++ {
		labels = append(labels, map[string]string{"the_label": fmt.Sprintf("label%d", l)})
	}

	for _, mn := range metricNames {
		for _, lv := range labels {
			counter, _ := exporter.Summaries.Get(mn, lv, "help")
			counter.Observe(float64(1.0))
		}
	}
}

func BenchmarkGatherSummary(b *testing.B) {
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
		c.observeSummary(exporter)
		runtime.GC()
		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, _ = prometheus.DefaultGatherer.Gather()
			}
		})
	}
}

func (gen metricGenerator) observeHistogram(exporter *Exporter) {
	metricNames := make([]string, 0, gen.metrics)
	for m := 0; m < gen.metrics; m++ {
		metricNames = append(metricNames, fmt.Sprintf("metric%d", m))
	}
	labels := make([]map[string]string, 0, gen.labels)
	for l := 0; l < gen.labels; l++ {
		labels = append(labels, map[string]string{"the_label": fmt.Sprintf("label%d", l)})
	}

	for _, mn := range metricNames {
		for _, lv := range labels {
			counter, _ := exporter.Histograms.Get(mn, lv, "help", nil)
			counter.Observe(float64(1.0))
		}
	}
}

func BenchmarkGatherHistogram(b *testing.B) {
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
		c.observeHistogram(exporter)
		runtime.GC()
		b.Run(fmt.Sprintf("m %d l %d", c.metrics, c.labels), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, _ = prometheus.DefaultGatherer.Gather()
			}
		})
	}
}
