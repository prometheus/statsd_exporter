// Copyright 2020 The Prometheus Authors
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
	"testing"

	"github.com/go-kit/kit/log"

	"github.com/prometheus/statsd_exporter/pkg/line"
)

var (
	// just a grab bag of mixed formats, valid, invalid
	mixedLines = []string{
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

	// The format specific lines have only one line each so the benchmark accurately reflects the time taken to process one line
	statsdLine           = "foo1:2|c"
	statsdInvalidLine    = "foo1:2|c||"
	dogStatsdLine        = "foo1:100|c|#tag1:bar,tag2:baz"
	dogStatsdInvalidLine = "foo3:100|c|#09digits:0,tag.with.dots:1"
	signalFxLine         = "foo1.[foo=bar1,dim=val1]test:1|g"
	signalFxInvalidLine  = "foo1.[foo=bar1,dim=val1test:1|g"
	influxDbLine         = "foo1,tag1=bar,tag2=baz:100|c"
	influxDbInvalidLine  = "foo3,tag1=bar,tag2:100|c"

	logger = log.NewNopLogger()
)

func benchmarkLinesToEvents(times int, b *testing.B, input []string) {
	// always report allocations since this is a hot path
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		for i := 0; i < times; i++ {
			for _, l := range input {
				line.LineToEvents(l, *sampleErrors, samplesReceived, tagErrors, tagsReceived, logger)
			}
		}
	}
}

func benchmarkLineToEvents(b *testing.B, inputLine string) {
	// always report allocations since this is a hot path
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		line.LineToEvents(inputLine, *sampleErrors, samplesReceived, tagErrors, tagsReceived, logger)
	}
}

// Mixed statsd formats
func BenchmarkLineToEventsMixed1(b *testing.B) {
	benchmarkLinesToEvents(1, b, mixedLines)
}
func BenchmarkLineToEventsMixed5(b *testing.B) {
	benchmarkLinesToEvents(5, b, mixedLines)
}
func BenchmarkLineToEventsMixed50(b *testing.B) {
	benchmarkLinesToEvents(50, b, mixedLines)
}

// Individual format benchmarks
// Valid Lines
func BenchmarkLineToEventsStatsd(b *testing.B) {
	benchmarkLineToEvents(b, statsdLine)
}
func BenchmarkLineToEventsDogStatsd(b *testing.B) {
	benchmarkLineToEvents(b, dogStatsdLine)
}
func BenchmarkLineToEventsSignalFx(b *testing.B) {
	benchmarkLineToEvents(b, signalFxLine)
}
func BenchmarkLineToEventsInfluxDb(b *testing.B) {
	benchmarkLineToEvents(b, influxDbLine)
}

// Invalid lines
func BenchmarkLineToEventsStatsdInvalid(b *testing.B) {
	benchmarkLineToEvents(b, statsdInvalidLine)
}
func BenchmarkLineToEventsDogStatsdInvalid(b *testing.B) {
	benchmarkLineToEvents(b, dogStatsdInvalidLine)
}
func BenchmarkLineToEventsSignalFxInvalid(b *testing.B) {
	benchmarkLineToEvents(b, signalFxInvalidLine)
}
func BenchmarkLineToEventsInfluxDbInvalid(b *testing.B) {
	benchmarkLineToEvents(b, influxDbInvalidLine)
}
