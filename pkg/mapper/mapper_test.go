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

package mapper

import (
	"testing"
)

type mappings map[string]struct {
	name       string
	labels     map[string]string
	quantiles  []metricObjective
	notPresent bool
}

func TestMetricMapperYAML(t *testing.T) {
	scenarios := []struct {
		config    string
		configBad bool
		mappings  mappings
	}{
		// Empty config.
		{},
		// Config with several mapping definitions.
		{
			config: `---
mappings:
- match: test.dispatcher.*.*.*
  name: "dispatch_events"
  labels: 
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher.*.*.*
  name: "host_dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: request_time.*.*.*.*.*.*.*.*.*.*.*.*
  name: "tyk_http_request"
  labels:
    method_and_path: "${1}"
    response_code: "${2}"
    apikey: "${3}"
    apiversion: "${4}"
    apiname: "${5}"
    apiid: "${6}"
    ipv4_t1: "${7}"
    ipv4_t2: "${8}"
    ipv4_t3: "${9}"
    ipv4_t4: "${10}"
    orgid: "${11}"
    oauthid: "${12}"
- match: "*.*"
  name: "catchall"
  labels:
    first: "$1"
    second: "$2"
    third: "$3"
    job: "$1-$2-$3"
- match: (.*)\.(.*)-(.*)\.(.*)
  match_type: regex
  name: "proxy_requests_total"
  labels:
    job: "$1"
    protocol: "$2"
    endpoint: "$3"
    result: "$4"

  `,
			mappings: mappings{
				"test.dispatcher.FooProcessor.send.succeeded": {
					name: "dispatch_events",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"result":    "succeeded",
						"job":       "test_dispatcher",
					},
				},
				"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded": {
					name: "host_dispatch_events",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"result":    "succeeded",
						"job":       "test_dispatcher",
					},
				},
				"request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.": {
					name: "tyk_http_request",
					labels: map[string]string{
						"method_and_path": "get/threads/1/posts",
						"response_code":   "200",
						"apikey":          "00000000",
						"apiversion":      "nonversioned",
						"apiname":         "discussions",
						"apiid":           "a11bbcdf0ac64ec243658dc64b7100fb",
						"ipv4_t1":         "172",
						"ipv4_t2":         "20",
						"ipv4_t3":         "0",
						"ipv4_t4":         "1",
						"orgid":           "12ba97b7eaa1a50001000001",
						"oauthid":         "",
					},
				},
				"foo.bar": {
					name: "catchall",
					labels: map[string]string{
						"first":  "foo",
						"second": "bar",
						"third":  "",
						"job":    "foo-bar-",
					},
				},
				"foo.bar.baz": {},
				"proxy-1.http-goober.success": {
					name: "proxy_requests_total",
					labels: map[string]string{
						"job":      "proxy-1",
						"protocol": "http",
						"endpoint": "goober",
						"result":   "success",
					},
				},
			},
		},
		// Config with bad regex reference.
		{
			config: `---
mappings:
- match: test.*
  name: "name"
  labels:
    label: "$1_foo"
  `,
			mappings: mappings{
				"test.a": {
					name: "name",
					labels: map[string]string{
						"label": "",
					},
				},
			},
		},
		// Config with good regex reference.
		{
			config: `
mappings:
- match: test.*
  name: "name"
  labels:
    label: "${1}_foo"
  `,
			mappings: mappings{
				"test.a": {
					name: "name",
					labels: map[string]string{
						"label": "a_foo",
					},
				},
			},
		},
		// Config with bad metric line.
		{
			config: `---
mappings:
- match: bad--metric-line.*.*
  name: "foo"
  labels: {}
  `,
			configBad: true,
		},
		// Config with dynamic metric name.
		{
			config: `---
mappings:
- match: test1.*.*
  name: "$1"
  labels: {}
- match: test2.*.*
  name: "${1}_$2"
  labels: {}
- match: test3\.(\w+)\.(\w+)
  match_type: regex
  name: "${2}_$1"
  labels: {}
  `,
			mappings: mappings{
				"test1.total_requests.count": {
					name: "total_requests",
				},
				"test2.total_requests.count": {
					name: "total_requests_count",
				},
				"test3.total_requests.count": {
					name: "count_total_requests",
				},
			},
		},
		// Config with bad metric name.
		{
			config: `---
mappings:
- match: test.*.*
  name: "0foo"
  labels: {}
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
			mappings: mappings{},
		},
		// Config without a trailing newline.
		{
			config: `mappings:
- match: test.*
  name: "name"
  labels:
    label: "${1}_foo"`,
			mappings: mappings{
				"test.a": {
					name: "name",
					labels: map[string]string{
						"label": "a_foo",
					},
				},
			},
		},
		// Config with an improperly escaped *.
		{
			config: `
mappings:
- match: *.test.*
  name: "name"
  labels:
    label: "${1}_foo"`,
			configBad: true,
		},
		// Config with a properly escaped *.
		{
			config: `
mappings:
- match: "*.test.*"
  name: "name"
  labels:
    label: "${2}_foo"`,
			mappings: mappings{
				"foo.test.a": {
					name: "name",
					labels: map[string]string{
						"label": "a_foo",
					},
				},
			},
		},
		// Config with good timer type.
		{
			config: `---
mappings:
- match: test.*.*
  timer_type: summary
  name: "foo"
  labels: {}
  quantiles:
    - quantile: 0.42
      error: 0.04
    - quantile: 0.7
      error: 0.002
  `,
			mappings: mappings{
				"test.*.*": {
					name:   "foo",
					labels: map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
				},
			},
		},
		{
			config: `---
mappings:
- match: test1.*.*
  timer_type: summary
  name: "foo"
  labels: {}
  `,
			mappings: mappings{
				"test1.*.*": {
					name:   "foo",
					labels: map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.5, Error: 0.05},
						{Quantile: 0.9, Error: 0.01},
						{Quantile: 0.99, Error: 0.001},
					},
				},
			},
		},
		// Config with bad timer type.
		{
			config: `---
mappings:
- match: test.*.*
  timer_type: wrong
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		// Config with good metric type.
		{
			config: `---
mappings:
- match: test.*.*
  match_metric_type: counter
  name: "foo"
  labels: {}
    `,
		},
		// Config with bad metric type matcher.
		{
			config: `---
mappings:
- match: test.*.*
  match_metric_type: wrong
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		//Config with uncompilable regex.
		{
			config: `---
mappings:
- match: "*\\.foo"
  match_type: regex
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		//Config with non-matched metric.
		{
			config: `---
mappings:
- match: foo.*.*
  timer_type: summary
  name: "foo"
  labels: {}
  `,
			mappings: mappings{
				"test.1.2": {
					name:       "test_1_2",
					labels:     map[string]string{},
					notPresent: true,
				},
			},
		},
		//Config with no name.
		{
			config: `---
mappings:
- match: *\.foo
  match_type: regex
  labels:
    bar: "foo"
    `,
			configBad: true,
		},
		// Example from the README.
		{
			config: `
mappings:
- match: test.dispatcher.*.*.*
  name: "dispatcher_events_total"
  labels:
    processor: "$1"
    action: "$2"
    outcome: "$3"
    job: "test_dispatcher"
- match: "*.signup.*.*"
  name: "signup_events_total"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
`,
			mappings: mappings{
				"test.dispatcher.FooProcessor.send.success": {
					name: "dispatcher_events_total",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"outcome":   "success",
						"job":       "test_dispatcher",
					},
				},
				"foo_product.signup.facebook.failure": {
					name: "signup_events_total",
					labels: map[string]string{
						"provider": "facebook",
						"outcome":  "failure",
						"job":      "foo_product_server",
					},
				},
				"test.web-server.foo.bar": {
					name:   "test_web_server_foo_bar",
					labels: map[string]string{},
				},
			},
		},
		// Config that drops all.
		{
			config: `mappings:
- match: .
  match_type: regex
  name: "drop"
  action: drop`,
			mappings: mappings{
				"test.a": {},
				"abc":    {},
			},
		},
		// Config that has a catch-all to drop all.
		{
			config: `mappings:
- match: web.*
  name: "web"
  labels:
    site: "$1"
- match: .
  match_type: regex
  name: "drop"
  action: drop`,
			mappings: mappings{
				"test.a": {},
				"web.localhost": {
					name: "web",
					labels: map[string]string{
						"site": "localhost",
					},
				},
			},
		},
		//Config with backtracking
		{
			config: `mappings:
- match: foo.*.ccc
  name: "fooc"
  labels: {}
- match: foo.bbb.aaa
  name: "foob"
  labels: {}
  `,
			mappings: mappings{
				"foo.bbb.ccc": {
					name:   "fooc",
					labels: map[string]string{},
				},
				"foo.ddd.ccc": {
					name:   "fooc",
					labels: map[string]string{},
				},
				"foo.bbb.aaa": {
					name:   "foob",
					labels: map[string]string{},
				},
			},
		},
	}

	mapper := MetricMapper{}
	for i, scenario := range scenarios {
		err := mapper.InitFromYAMLString(scenario.config)
		if err != nil && !scenario.configBad {
			t.Fatalf("%d. Config load error: %s %s", i, scenario.config, err)
		}
		if err == nil && scenario.configBad {
			t.Fatalf("%d. Expected bad config, but loaded ok: %s", i, scenario.config)
		}

		var dummyMetricType MetricType = ""
		for metric, mapping := range scenario.mappings {
			m, labels, present := mapper.GetMapping(metric, dummyMetricType)
			if present && mapping.name != "" && m.Name != mapping.name {
				t.Fatalf("%d.%q: Expected name %v, got %v", i, metric, m.Name, mapping.name)
			}
			if mapping.notPresent && present {
				t.Fatalf("%d.%q: Expected metric to not be present", i, metric)
			}
			if len(labels) != len(mapping.labels) {
				t.Fatalf("%d.%q: Expected %d labels, got %d", i, metric, len(mapping.labels), len(labels))
			}
			for label, value := range labels {
				if mapping.labels[label] != value {
					t.Fatalf("%d.%q: Expected labels %v, got %v", i, metric, mapping, labels)
				}
			}

			if len(mapping.quantiles) != 0 {
				if len(mapping.quantiles) != len(m.Quantiles) {
					t.Fatalf("%d.%q: Expected %d quantiles, got %d", i, metric, len(mapping.quantiles), len(m.Quantiles))
				}
				for i, quantile := range mapping.quantiles {
					if quantile.Quantile != m.Quantiles[i].Quantile {
						t.Fatalf("%d.%q: Expected quantile %v, got %v", i, metric, m.Quantiles[i].Quantile, quantile.Quantile)
					}
					if quantile.Error != m.Quantiles[i].Error {
						t.Fatalf("%d.%q: Expected Error margin %v, got %v", i, metric, m.Quantiles[i].Error, quantile.Error)
					}
				}
			}
		}
	}
}

/*func TestRPS(t *testing.T) {
	scenarios := []struct {
		config    string
		configBad bool
		mappings  mappings
	}{
		// Empty config.
		{},
		// Config with several mapping definitions.
		{
			config: `---
mappings:
- match: test.dispatcher.*.*.*
  name: "dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: test.my-dispatch-host01.name.dispatcher.*.*.*
  name: "host_dispatch_events"
  labels:
    processor: "$1"
    action: "$2"
    result: "$3"
    job: "test_dispatcher"
- match: request_time.*.*.*.*.*.*.*.*.*.*.*.*
  name: "tyk_http_request"
  labels:
    method_and_path: "${1}"
    response_code: "${2}"
    apikey: "${3}"
    apiversion: "${4}"
    apiname: "${5}"
    apiid: "${6}"
    ipv4_t1: "${7}"
    ipv4_t2: "${8}"
    ipv4_t3: "${9}"
    ipv4_t4: "${10}"
    orgid: "${11}"
    oauthid: "${12}"
- match: "*.*"
  name: "catchall"
  labels:
    first: "$1"
    second: "$2"
    third: "$3"
    job: "-"
  `,
			mappings: mappings{
				"test.dispatcher.FooProcessor.send.succeeded": {
					name: "dispatch_events",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"result":    "succeeded",
						"job":       "test_dispatcher",
					},
				},
				"test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded": {
					name: "host_dispatch_events",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"result":    "succeeded",
						"job":       "test_dispatcher",
					},
				},
				"request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.": {
					name: "tyk_http_request",
					labels: map[string]string{
						"method_and_path": "get/threads/1/posts",
						"response_code":   "200",
						"apikey":          "00000000",
						"apiversion":      "nonversioned",
						"apiname":         "discussions",
						"apiid":           "a11bbcdf0ac64ec243658dc64b7100fb",
						"ipv4_t1":         "172",
						"ipv4_t2":         "20",
						"ipv4_t3":         "0",
						"ipv4_t4":         "1",
						"orgid":           "12ba97b7eaa1a50001000001",
						"oauthid":         "",
					},
				},
				"foo.bar": {
					name: "catchall",
					labels: map[string]string{
						"first":  "foo",
						"second": "bar",
						"third":  "",
						"job":    "-",
					},
				},
				"foo.bar.baz": {},
			},
		},
	}

	mapper := MetricMapper{}
	for i, scenario := range scenarios {
		err := mapper.InitFromYAMLString(scenario.config)
		if err != nil && !scenario.configBad {
			t.Fatalf("%d. Config load error: %s %s", i, scenario.config, err)
		}
		if err == nil && scenario.configBad {
			t.Fatalf("%d. Expected bad config, but loaded ok: %s", i, scenario.config)
		}

		var dummyMetricType MetricType = ""
		start := int32(time.Now().Unix())
		for j := 1; j < 100000; j++ {
			for metric, mapping := range scenario.mappings {
				m, labels, present := mapper.GetMapping(metric, dummyMetricType)
				if present && mapping.name != "" && m.Name != mapping.name {
					t.Fatalf("%d.%q: Expected name %v, got %v", i, metric, m.Name, mapping.name)
				}
				if mapping.notPresent && present {
					t.Fatalf("%d.%q: Expected metric to not be present", i, metric)
				}
				if len(labels) != len(mapping.labels) {
					t.Fatalf("%d.%q: Expected %d labels, got %d", i, metric, len(mapping.labels), len(labels))
				}
				for label, value := range labels {
					if mapping.labels[label] != value {
						t.Fatalf("%d.%q: Expected labels %v, got %v", i, metric, mapping, labels)
					}
				}
			}
		}
		fmt.Println("finished in", int32(time.Now().Unix())-start)
	}
}*/

func TestAction(t *testing.T) {
	scenarios := []struct {
		config         string
		configBad      bool
		expectedAction ActionType
	}{
		{
			// no action set
			config: `---
mappings:
- match: test.*.*
  name: "foo"
`,
			configBad:      false,
			expectedAction: ActionTypeMap,
		},
		{
			// map action set
			config: `---
mappings:
- match: test.*.*
  name: "foo"
  action: map
`,
			configBad:      false,
			expectedAction: ActionTypeMap,
		},
		{
			// drop action set
			config: `---
mappings:
- match: test.*.*
  name: "foo"
  action: drop
`,
			configBad:      false,
			expectedAction: ActionTypeDrop,
		},
		{
			// invalid action set
			config: `---
mappings:
- match: test.*.*
  name: "foo"
  action: xyz
`,
			configBad:      true,
			expectedAction: ActionTypeDrop,
		},
	}

	for i, scenario := range scenarios {
		mapper := MetricMapper{}
		err := mapper.InitFromYAMLString(scenario.config)
		if err != nil && !scenario.configBad {
			t.Fatalf("%d. Config load error: %s %s", i, scenario.config, err)
		}
		if err == nil && scenario.configBad {
			t.Fatalf("%d. Expected bad config, but loaded ok: %s", i, scenario.config)
		}

		if !scenario.configBad {
			a := mapper.Mappings[0].Action
			if scenario.expectedAction != a {
				t.Fatalf("%d: Expected action %v, got %v", i, scenario.expectedAction, a)
			}
		}
	}
}
