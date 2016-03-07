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
	"reflect"
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
					labels:     map[string]string{},
				},
			},
		}, {
			name: "simple gauge",
			in:   "foo:3|g",
			out: Events{
				&GaugeEvent{
					metricName: "foo",
					value:      3,
					labels:     map[string]string{},
				},
			},
		}, {
			name: "simple timer",
			in:   "foo:200|ms",
			out: Events{
				&TimerEvent{
					metricName: "foo",
					value:      200,
					labels:     map[string]string{},
				},
			},
		}, {
			name: "datadog tag extension",
			in:   "foo:100|c|#tag1:bar,tag2:baz,tag3,tag4",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      100,
					labels:     map[string]string{"tag1": "bar", "tag2": "baz", "tag3": ".", "tag4": "."},
				},
			},
		}, {
			name: "datadog tag extension with # in all keys (as sent by datadog php client)",
			in:   "foo:100|c|#tag1:bar,#tag2:baz,#tag3,#tag4",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      100,
					labels:     map[string]string{"tag1": "bar", "tag2": "baz", "tag3": ".", "tag4": "."},
				},
			},
		}, {
			name: "datadog tag extension with tags unsupported by prometheus",
			in:   "foo:100|c|#09digits:0,tag.with.dots,tag_with_empty_value:",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      100,
					labels:     map[string]string{"_09digits": "0", "tag_with_dots": ".", "tag_with_empty_value": "."},
				},
			},
		}, {
			name: "datadog tag extension with sampling",
			in:   "foo:100|c|@0.1|#tag1:bar,#tag2,#tag3:baz",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      1000,
					labels:     map[string]string{"tag1": "bar", "tag2": ".", "tag3": "baz"},
				},
			},
		}, {
			name: "combined multiline metrics",
			in:   "foo:200|ms:300|ms:5|c|@0.1:6|g\nbar:1|c:5|ms",
			out: Events{
				&TimerEvent{
					metricName: "foo",
					value:      200,
					labels:     map[string]string{},
				},
				&TimerEvent{
					metricName: "foo",
					value:      300,
					labels:     map[string]string{},
				},
				&CounterEvent{
					metricName: "foo",
					value:      50,
					labels:     map[string]string{},
				},
				&GaugeEvent{
					metricName: "foo",
					value:      6,
					labels:     map[string]string{},
				},
				&CounterEvent{
					metricName: "bar",
					value:      1,
					labels:     map[string]string{},
				},
				&TimerEvent{
					metricName: "bar",
					value:      5,
					labels:     map[string]string{},
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
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      1,
					labels:     map[string]string{},
				},
			},
		}, {
			name: "zero sampling factor",
			in:   "foo:2|c|@0",
			out: Events{
				&CounterEvent{
					metricName: "foo",
					value:      2,
					labels:     map[string]string{},
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
			t.Fatalf("%d. Expected %d events, got %d in scenario '%s'", i, len(scenario.out), len(actual), scenario.name)
		}

		for j, expected := range scenario.out {
			if !reflect.DeepEqual(&expected, &actual[j]) {
				t.Fatalf("%d.%d. Expected %#v, got %#v in scenario '%s'", i, j, expected, actual[j], scenario.name)
			}
		}
	}
}
