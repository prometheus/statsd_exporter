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
	"testing"
)

func TestMetricMapper(t *testing.T) {
	scenarios := []struct {
		config    string
		configBad bool
		mappings  map[string]map[string]string
	}{
		// Empty config.
		{},
		// Config with several mapping definitions.
		{
			config: `
				test.dispatcher.*.*.*
				name="dispatch_events"
				processor="$1"
				action="$2"
				result="$3"
				job="test_dispatcher"

				test.my-dispatch-host01.name.dispatcher.*.*.*
				name="host_dispatch_events"
				processor="$1"
				action="$2"
				result="$3"
				job="test_dispatcher"

				*.*
				name="catchall"
				first="$1"
				second="$2"
				third="$3"
				job="$1-$2-$3"
			`,
			mappings: map[string]map[string]string{
				"test.dispatcher.FooProcessor.send.succeeded": map[string]string{
					"name":      "dispatch_events",
					"processor": "FooProcessor",
					"action":    "send",
					"result":    "succeeded",
					"job":       "test_dispatcher",
				},
				"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded": map[string]string{
					"name":      "host_dispatch_events",
					"processor": "FooProcessor",
					"action":    "send",
					"result":    "succeeded",
					"job":       "test_dispatcher",
				},
				"foo.bar": map[string]string{
					"name":   "catchall",
					"first":  "foo",
					"second": "bar",
					"third":  "",
					"job":    "foo-bar-",
				},
				"foo.bar.baz": map[string]string{},
			},
		},
		// Config with bad regex reference.
		{
			config: `
				test.*
				name="name"
				label="$1_foo"
			`,
			mappings: map[string]map[string]string{
				"test.a": map[string]string{
					"name":  "name",
					"label": "",
				},
			},
		},
		// Config with good regex reference.
		{
			config: `
				test.*
				name="name"
				label="${1}_foo"
			`,
			mappings: map[string]map[string]string{
				"test.a": map[string]string{
					"name":  "name",
					"label": "a_foo",
				},
			},
		},
		// Config with bad metric line.
		{
			config: `
				bad--metric-line.*.*
				name="foo"
			`,
			configBad: true,
		},
		// Config with bad label line.
		{
			config: `
				test.*.*
				name=foo
			`,
			configBad: true,
		},
		// Config with bad label line.
		{
			config: `
				test.*.*
				name="foo-name"
			`,
			configBad: true,
		},
		// Config with bad metric name.
		{
			config: `
				test.*.*
				name="0foo"
			`,
			configBad: true,
		},
		// Config without a terminating newline.
		{
			config: `
				test.*
				name="name"
				label="foo"`,
			mappings: map[string]map[string]string{
				"test.a": map[string]string{
					"name":  "name",
					"label": "foo",
				},
			},
		},
	}

	mapper := metricMapper{}
	for i, scenario := range scenarios {
		err := mapper.initFromString(scenario.config)
		if err != nil && !scenario.configBad {
			t.Fatalf("%d. Config load error: %s", i, err)
		}
		if err == nil && scenario.configBad {
			t.Fatalf("%d. Expected bad config, but loaded ok", i)
		}

		for metric, mapping := range scenario.mappings {
			labels, present := mapper.getMapping(metric)
			if len(labels) == 0 && present {
				t.Fatalf("%d.%q: Expected metric to not be present", i, metric)
			}
			if len(labels) != len(mapping) {
				t.Fatalf("%d.%q: Expected %d labels, got %d", i, metric, len(mapping), len(labels))
			}
			for label, value := range labels {
				if mapping[label] != value {
					t.Fatalf("%d.%q: Expected labels %v, got %v", i, metric, mapping, labels)
				}
			}
		}
	}
}
