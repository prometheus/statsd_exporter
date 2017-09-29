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

func TestMetricMapperYAML(t *testing.T) {
	scenarios := []struct {
		config    string
		configBad bool
		mappings  map[string]map[string]string
	}{
		// Empty config.
		{},
		// Config with several mapping definitions.
		{
			config: `---
mappings:
- match: test.dispatcher.*.*.*
  labels: 
    name: "dispatch_events"
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher.*.*.*
  labels:
    name: "host_dispatch_events"
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: "*.*"
  labels:
    name: "catchall"
    first: "$1"
    second: "$2"
    third: "$3"
    job: "$1-$2-$3"
- match: (.*)\.(.*)-(.*)\.(.*)
  match_type: regex
  labels:
    name: "proxy_requests_total"
    job: "$1"
    protocol: "$2"
    endpoint: "$3"
    result: "$4"

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
				"proxy-1.http-goober.success": map[string]string{
					"name":     "proxy_requests_total",
					"job":      "proxy-1",
					"protocol": "http",
					"endpoint": "goober",
					"result":   "success",
				},
			},
		},
		// Config with bad regex reference.
		{
			config: `---
mappings:
- match: test.*
  labels:
    name: "name"
    label: "$1_foo"
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
mappings:
- match: test.*
  labels:
    name: "name"
    label: "${1}_foo"
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
			config: `---
mappings:
- match: bad--metric-line.*.*
  labels:
    name: "foo"
  `,
			configBad: true,
		},
		// Config with bad metric name.
		{
			config: `---
mappings:
- match: test.*.*
  labels:
    name: "0foo"
  `,
			configBad: true,
		},
		// Config with no metric name.
		{
			config: `---
mappings:
- match: test.*.*
  labels:
    this: "$1"
  `,
			configBad: true,
		},
		// Config with no mappings.
		{
			config:   ``,
			mappings: map[string]map[string]string{},
		},
		// Config with good timer type.
		{
			config: `---
mappings:
- match: test.*.*
  timer_type: summary
  labels:
    name: "foo"
  `,
			mappings: map[string]map[string]string{
				"test.*.*": map[string]string{
					"name": "foo",
				},
			},
		},
		// Config with bad timer type.
		{
			config: `---
mappings:
- match: test.*.*
  timer_type: wrong
  labels:
    name: "foo"
    `,
			configBad: true,
		},
		//Config with uncompilable regex.
		{
			config: `---
mappings:
- match: *\.foo
  match_type: regex
  labels:
    name: "foo"
    `,
			configBad: true,
		},
	}

	mapper := metricMapper{}
	for i, scenario := range scenarios {
		err := mapper.initFromYAMLString(scenario.config)
		if err != nil && !scenario.configBad {
			t.Fatalf("%d. Config load error: %s %s", i, scenario.config, err)
		}
		if err == nil && scenario.configBad {
			t.Fatalf("%d. Expected bad config, but loaded ok: %s", i, scenario.config)
		}

		for metric, mapping := range scenario.mappings {
			_, labels, present := mapper.getMapping(metric)
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
