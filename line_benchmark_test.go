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

func benchmarkLineToEvents(times int, b *testing.B) {
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
	logger := log.NewNopLogger()

	for n := 0; n < b.N; n++ {

		for i := 0; i < times; i++ {
			for _, l := range input {
				line.LineToEvents(l, *sampleErrors, samplesReceived, tagErrors, tagsReceived, logger)
			}
		}
	}
}

func BenchmarkLineToEvents1(b *testing.B) {
	benchmarkLineToEvents(1, b)
}
func BenchmarkLineToEvents5(b *testing.B) {
	benchmarkLineToEvents(5, b)
}
func BenchmarkLineToEvents50(b *testing.B) {
	benchmarkLineToEvents(50, b)
}
