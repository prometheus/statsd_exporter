// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
