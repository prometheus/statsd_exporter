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
)

func TestHandlePacket(t *testing.T) {
	scenarios := []struct {
		name string
		in   string
		out  Events
	}{
		{
			name: "empty",
		}, {
			name: "simple counter",
			in:   "foo:2|c",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      2,
				},
			},
		}, {
			name: "simple gauge",
			in:   "foo:3|g",
			out: Events{
				&GaugeEvent{
					metricName: "foo",
					value:      3,
				},
			},
		}, {
			name: "simple timer",
			in:   "foo:200|ms",
			out: Events{
				&TimerEvent{
					metricName: "foo",
					value:      200,
				},
			},
		}, {
			name: "combined multiline metrics",
			in:   "foo:200|ms:300|ms:5|c|@0.1:6|g\nbar:1|c:5|ms",
			out: Events{
				&TimerEvent{
					metricName: "foo",
					value:      200,
				},
				&TimerEvent{
					metricName: "foo",
					value:      300,
				},
				&CounterEvent{
					metricName: "foo",
					value:      50,
				},
				&GaugeEvent{
					metricName: "foo",
					value:      6,
				},
				&CounterEvent{
					metricName: "bar",
					value:      1,
				},
				&TimerEvent{
					metricName: "bar",
					value:      5,
				},
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
		}, {
			name: "zero sampling factor",
			in:   "foo:2|c|@0",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      2,
				},
			},
		}, {
			name: "illegal stat type",
			in:   "foo:2|t",
		},
	}

	l := StatsDListener{}
	events := make(chan Events, 32)
	for i, scenario := range scenarios {
		l.handlePacket([]byte(scenario.in), events)

		// Flatten actual events.
		actual := Events{}
		for i := 0; i < len(events); i++ {
			actual = append(actual, <-events...)
		}

		if len(actual) != len(scenario.out) {
			t.Fatalf("%d. Expected %d events, got %d", i, len(scenario.out), len(actual))
		}

		for j, expected := range scenario.out {
			if fmt.Sprintf("%v", actual[j]) != fmt.Sprintf("%v", expected) {
				t.Fatalf("%d.%d. Expected %v, got %v", i, j, actual[j], expected)
			}
		}
	}
}
