// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
		// Config with bad metric line.
		{
			config: `
				bad-metric-line.*.*
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
