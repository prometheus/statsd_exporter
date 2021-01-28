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
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type mappings []struct {
	statsdMetric string
	name         string
	labels       map[string]string
	quantiles    []metricObjective
	notPresent   bool
	ttl          time.Duration
	metricType   MetricType
	maxAge       time.Duration
	ageBuckets   uint32
	bufCap       uint32
	buckets      []float64
}

func TestMetricMapperYAML(t *testing.T) {
	scenarios := []struct {
		testName  string
		config    string
		configBad bool
		mappings  mappings
	}{
		{
			testName: "Empty config",
		},
		{
			testName: "Config with several mapping definitions",
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
				{
					statsdMetric: "test.dispatcher.FooProcessor.send.succeeded",
					name:         "dispatch_events",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"result":    "succeeded",
						"job":       "test_dispatcher",
					},
				},
				{
					statsdMetric: "test.my-dispatch-host01.name.dispatcher.FooProcessor.send.succeeded",
					name:         "host_dispatch_events",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"result":    "succeeded",
						"job":       "test_dispatcher",
					},
				},
				{
					statsdMetric: "request_time.get/threads/1/posts.200.00000000.nonversioned.discussions.a11bbcdf0ac64ec243658dc64b7100fb.172.20.0.1.12ba97b7eaa1a50001000001.",
					name:         "tyk_http_request",
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
				{
					statsdMetric: "foo.bar",
					name:         "catchall",
					labels: map[string]string{
						"first":  "foo",
						"second": "bar",
						"third":  "",
						"job":    "foo-bar-",
					},
				},
				{
					statsdMetric: "foo.bar.baz",
				},
				{
					statsdMetric: "proxy-1.http-goober.success",
					name:         "proxy_requests_total",
					labels: map[string]string{
						"job":      "proxy-1",
						"protocol": "http",
						"endpoint": "goober",
						"result":   "success",
					},
				},
			},
		},
		{
			testName: "Config with backtracking",
			config: `
defaults:
  glob_disable_ordering: true
mappings:
- match: backtrack.*.bbb
  name: "testb"
  labels:
    label: "${1}_foo"
- match: backtrack.justatest.aaa
  name: "testa"
  labels:
    label: "${1}_foo"
  `,
			mappings: mappings{
				{
					statsdMetric: "backtrack.good.bbb",
					name:         "testb",
					labels: map[string]string{
						"label": "good_foo",
					},
				},
				{
					statsdMetric: "backtrack.justatest.bbb",
					name:         "testb",
					labels: map[string]string{
						"label": "justatest_foo",
					},
				},
				{
					statsdMetric: "backtrack.justatest.aaa",
					name:         "testa",
					labels: map[string]string{
						"label": "_foo",
					},
				},
			},
		},
		//Config with backtracking, the non-matched rule has star(s)
		// A metric like full.name.anothertest will first match full.name.* and then tries
		// to match *.dummy.* and then failed.
		// This test case makes sure the captures in the non-matched later rule
		// doesn't affect the captures in the first matched rule.
		{
			testName: "Config with backtracking, the non-matched rule has star(s)",
			config: `
defaults:
  glob_disable_ordering: false
mappings:
- match: '*.dummy.*'
  name: metric_one
  labels:
    system: $1
    attribute: $2
- match: 'full.name.*'
  name: metric_two
  labels:
    system: static
    attribute: $1
`,
			mappings: mappings{
				{
					statsdMetric: "whatever.dummy.test",
					name:         "metric_one",
					labels: map[string]string{
						"system":    "whatever",
						"attribute": "test",
					},
				},
				{
					statsdMetric: "full.name.anothertest",
					name:         "metric_two",
					labels: map[string]string{
						"system":    "static",
						"attribute": "anothertest",
					},
				},
			},
		},
		{
			testName: "Config with super sets, disables ordering",
			config: `
defaults:
  glob_disable_ordering: true
mappings:
- match: noorder.*.*
  name: "testa"
  labels:
    label: "${1}_foo"
- match: noorder.*.bbb
  name: "testb"
  labels:
    label: "${1}_foo"
- match: noorder.ccc.bbb
  name: "testc"
  labels:
    label: "ccc_foo"
  `,
			mappings: mappings{
				{
					statsdMetric: "noorder.good.bbb",
					name:         "testb",
					labels: map[string]string{
						"label": "good_foo",
					},
				},
				{
					statsdMetric: "noorder.ccc.bbb",
					name:         "testc",
					labels: map[string]string{
						"label": "ccc_foo",
					},
				},
			},
		},
		{
			testName: "Config with super sets, keeps ordering",
			config: `
defaults:
  glob_disable_ordering: false
mappings:
- match: order.*.*
  name: "testa"
  labels:
    label: "${1}_foo"
- match: order.*.bbb
  name: "testb"
  labels:
    label: "${1}_foo"
  `,
			mappings: mappings{
				{
					statsdMetric: "order.good.bbb",
					name:         "testa",
					labels: map[string]string{
						"label": "good_foo",
					},
				},
			},
		},
		{
			testName: "Config with bad regex reference",
			config: `---
mappings:
- match: test.*
  name: "name"
  labels:
    label: "$1_foo"
  `,
			mappings: mappings{
				{
					statsdMetric: "test.a",
					name:         "name",
					labels: map[string]string{
						"label": "",
					},
				},
			},
		},
		{
			testName: "Config with good regex reference",
			config: `
mappings:
- match: test.*
  name: "name"
  labels:
    label: "${1}_foo"
  `,
			mappings: mappings{
				{
					statsdMetric: "test.a",
					name:         "name",
					labels: map[string]string{
						"label": "a_foo",
					},
				},
			},
		},
		{
			testName: "Config with bad metric line",
			config: `---
mappings:
- match: bad--metric-line.*.*
  name: "foo"
  labels: {}
  `,
			configBad: true,
		},
		{
			testName: "Config with dynamic metric name",
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
				{
					statsdMetric: "test1.total_requests.count",
					name:         "total_requests",
				},
				{
					statsdMetric: "test2.total_requests.count",
					name:         "total_requests_count",
				},
				{
					statsdMetric: "test3.total_requests.count",
					name:         "count_total_requests",
				},
			},
		},
		{
			testName: "Config with bad metric name",
			config: `---
mappings:
- match: test.*.*
  name: "0foo"
  labels: {}
  `,
			configBad: true,
		},
		{
			testName: "Config with no metric name",
			config: `---
mappings:
- match: test.*.*
  labels:
    this: "$1"
  `,
			configBad: true,
		},
		{
			testName: "Config with no mappings",
			config:   ``,
			mappings: mappings{},
		},
		{
			testName: "Config without a trailing newline",
			config: `mappings:
- match: test.*
  name: "name"
  labels:
    label: "${1}_foo"`,
			mappings: mappings{
				{
					statsdMetric: "test.a",
					name:         "name",
					labels: map[string]string{
						"label": "a_foo",
					},
				},
			},
		},
		{
			testName: "Config with an improperly escaped *",
			config: `
mappings:
- match: *.test.*
  name: "name"
  labels:
    label: "${1}_foo"`,
			configBad: true,
		},
		{
			testName: "Config with a properly escaped *",
			config: `
mappings:
- match: "*.test.*"
  name: "name"
  labels:
    label: "${2}_foo"`,
			mappings: mappings{
				{
					statsdMetric: "foo.test.a",
					name:         "name",
					labels: map[string]string{
						"label": "a_foo",
					},
				},
			},
		},
		{
			testName: "Config with good observer type",
			config: `---
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  quantiles:
    - quantile: 0.42
      error: 0.04
    - quantile: 0.7
      error: 0.002
  `,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
				},
			},
		},
		{
			testName: "Config with good observer type and unused timer type",
			config: `---
mappings:
- match: test.*.*
  observer_type: summary
  timer_type: histogram
  name: "foo"
  labels: {}
  quantiles:
    - quantile: 0.42
      error: 0.04
    - quantile: 0.7
      error: 0.002
  `,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
				},
			},
		},
		{
			testName: "Config with good observertype and no defaults",
			config: `---
mappings:
- match: test1.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  `,
			mappings: mappings{
				{
					statsdMetric: "test1.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.5, Error: 0.05},
						{Quantile: 0.9, Error: 0.01},
						{Quantile: 0.99, Error: 0.001},
					},
				},
			},
		},
		{
			testName: "Config with good deprecated timer type",
			config: `---
mappings:
- match: test1.*.*
  timer_type: summary
  name: "foo"
  labels: {}
  `,
			mappings: mappings{
				{
					statsdMetric: "test1.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.5, Error: 0.05},
						{Quantile: 0.9, Error: 0.01},
						{Quantile: 0.99, Error: 0.001},
					},
				},
			},
		},
		{
			testName: "Config with bad observer type",
			config: `---
mappings:
- match: test.*.*
  observer_type: wrong
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		{
			testName: "Config with bad deprecated timer type",
			config: `---
mappings:
- match: test.*.*
  timer_type: wrong
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		{
			testName: "New style quantiles",
			config: `---
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  summary_options:
    quantiles:
      - quantile: 0.42
        error: 0.04
      - quantile: 0.7
        error: 0.002
  `,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
				},
			},
		},
		{
			testName: "Config with summary options",
			config: `---
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  summary_options:
    quantiles:
      - quantile: 0.42
        error: 0.04
      - quantile: 0.7
        error: 0.002
    max_age: 5m
    age_buckets: 2
    buf_cap: 1000
  `,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
					maxAge:     5 * time.Minute,
					ageBuckets: 2,
					bufCap:     1000,
				},
			},
		},
		{
			testName: "Config with default summary options",
			config: `---
defaults:
 summary_options:
   quantiles:
     - quantile: 0.42
       error: 0.04
     - quantile: 0.7
       error: 0.002
   max_age: 5m
   age_buckets: 2
   buf_cap: 1000
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
					maxAge:     5 * time.Minute,
					ageBuckets: 2,
					bufCap:     1000,
				},
			},
		},
		{
			testName: "Config with default summary options without quantiles",
			config: `---
defaults:
 summary_options:
   max_age: 5m
   age_buckets: 2
   buf_cap: 1000
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles:    defaultQuantiles,
					maxAge:       5 * time.Minute,
					ageBuckets:   2,
					bufCap:       1000,
				},
			},
		},
		{
			testName: "Config with default summary options overrides quantiles",
			config: `---
defaults:
  quantiles:
    - quantile: 0.9
      error: 0.1
    - quantile: 0.99
      error: 0.01
  summary_options:
    quantiles:
      - quantile: 0.42
        error: 0.04
      - quantile: 0.7
        error: 0.002
    max_age: 5m
    age_buckets: 2
    buf_cap: 1000
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
					maxAge:     5 * time.Minute,
					ageBuckets: 2,
					bufCap:     1000,
				},
			},
		},
		{
			testName: "Config that overrides default summary options",
			config: `---
defaults:
 summary_options:
   quantiles:
     - quantile: 0.042
       error: 0.4
     - quantile: 0.07
       error: 0.02
   max_age: 15m
   age_buckets: 3
   buf_cap: 100
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  summary_options:
    quantiles:
     - quantile: 0.42
       error: 0.04
     - quantile: 0.7
       error: 0.002
    max_age: 5m
    age_buckets: 2
    buf_cap: 1000
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
					maxAge:     5 * time.Minute,
					ageBuckets: 2,
					bufCap:     1000,
				},
			},
		},
		{
			testName: "Config that overrides default summary options and a default options mapping",
			config: `---
defaults:
 summary_options:
   quantiles:
     - quantile: 0.9
       error: 0.1
     - quantile: 0.99
       error: 0.01
   max_age: 15m
   age_buckets: 3
   buf_cap: 100
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  summary_options:
    quantiles:
     - quantile: 0.42
       error: 0.04
     - quantile: 0.7
       error: 0.002
    max_age: 5m
    age_buckets: 2
    buf_cap: 1000
- match: test_default.*.*
  observer_type: summary
  name: "foo_default"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.42, Error: 0.04},
						{Quantile: 0.7, Error: 0.002},
					},
					maxAge:     5 * time.Minute,
					ageBuckets: 2,
					bufCap:     1000,
				},
				{
					statsdMetric: "test_default.*.*",
					name:         "foo_default",
					labels:       map[string]string{},
					quantiles: []metricObjective{
						{Quantile: 0.9, Error: 0.1},
						{Quantile: 0.99, Error: 0.01},
					},
					maxAge:     15 * time.Minute,
					ageBuckets: 3,
					bufCap:     100,
				},
			},
		},
		{
			testName: "Config with histogram options",
			config: `---
mappings:
- match: test.*.*
  observer_type: histogram
  name: "foo"
  labels: {}
  histogram_options:
    buckets: [0.1, 1, 10, 100, 1000]
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					buckets:      []float64{0.1, 1, 10, 100, 1000},
				},
			},
		},
		{
			testName: "Config with default histogram options",
			config: `---
defaults:
  histogram_options:
    buckets: [0.1, 1, 10, 100, 1000]
mappings:
- match: test.*.*
  observer_type: histogram
  name: "foo"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					buckets:      []float64{0.1, 1, 10, 100, 1000},
				},
			},
		},
		{
			testName: "Config with default histogram options without buckets",
			config: `---
defaults:
  histogram_options:
    buckets: []
mappings:
- match: test.*.*
  observer_type: histogram
  name: "foo"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					buckets:      prometheus.DefBuckets,
				},
			},
		},
		{
			testName: "Config with default histogram options overrides buckets",
			config: `---
defaults:
  buckets: [0.2, 2, 20, 200, 2000]
  histogram_options:
    buckets: [0.1, 1, 10, 100, 1000]
mappings:
- match: test.*.*
  observer_type: histogram
  name: "foo"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					buckets:      []float64{0.1, 1, 10, 100, 1000},
				},
			},
		},
		{
			testName: "Config that overrides default histogram configuration",
			config: `---
defaults:
  histogram_options:
    buckets: [0.2, 2, 20, 200, 2000]
mappings:
- match: test.*.*
  observer_type: histogram
  name: "foo"
  labels: {}
  histogram_options:
    buckets: [0.1, 1, 10, 100, 1000]
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					buckets:      []float64{0.1, 1, 10, 100, 1000},
				},
			},
		},
		{
			testName: "Config that overrides default histogram configuration and a default options mapping",
			config: `---
defaults:
  histogram_options:
    buckets: [0.2, 2, 20, 200]
mappings:
- match: test.*.*
  observer_type: histogram
  name: "foo"
  labels: {}
  histogram_options:
    buckets: [0.1, 1, 10, 100, 1000]
- match: test_default.*.*
  observer_type: histogram
  name: "foo_default"
  labels: {}
`,
			mappings: mappings{
				{
					statsdMetric: "test.*.*",
					name:         "foo",
					labels:       map[string]string{},
					buckets:      []float64{0.1, 1, 10, 100, 1000},
				},
				{
					statsdMetric: "test_default.*.*",
					name:         "foo_default",
					labels:       map[string]string{},
					buckets:      []float64{0.2, 2, 20, 200},
				},
			},
		},
		{
			testName: "Duplicate quantiles are bad",
			config: `---
mappings:
- match: test.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  quantiles:
    - quantile: 0.42
      error: 0.04
  summary_options:
    quantiles:
      - quantile: 0.42
        error: 0.04
  `,
			configBad: true,
		},
		{
			testName: "Config with good metric type",
			config: `---
mappings:
- match: test.*.*
  match_metric_type: counter
  name: "foo"
  labels: {}
    `,
		},
		{
			testName: "Config with good metric type observer",
			config: `---
mappings:
- match: test.*.*
  match_metric_type: observer
  name: "foo"
  labels: {}
    `,
		},
		{
			testName: "Config with good metric type timer",
			config: `---
mappings:
- match: test.*.*
  match_metric_type: timer
  name: "foo"
  labels: {}
    `,
		},
		{
			testName: "Config with bad metric type matcher",
			config: `---
mappings:
- match: test.*.*
  match_metric_type: wrong
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		{
			testName: "Config with multiple explicit metric types",
			config: `---
mappings:
- match: test.foo.*
  name: "test_foo_sum"
  match_metric_type: counter
- match: test.foo.*
  name: "test_foo_current"
  match_metric_type: gauge
    `,
			mappings: mappings{
				{
					statsdMetric: "test.foo.test",
					name:         "test_foo_sum",
					metricType:   MetricTypeCounter,
				},
				{
					statsdMetric: "test.foo.test",
					name:         "test_foo_current",
					metricType:   MetricTypeGauge,
				},
			},
		},
		{
			testName: "Config with uncompilable regex",
			config: `---
mappings:
- match: "*\\.foo"
  match_type: regex
  name: "foo"
  labels: {}
    `,
			configBad: true,
		},
		{
			testName: "Config with non-matched metric",
			config: `---
mappings:
- match: foo.*.*
  observer_type: summary
  name: "foo"
  labels: {}
  `,
			mappings: mappings{
				{
					statsdMetric: "test.1.2",
					name:         "test_1_2",
					labels:       map[string]string{},
					notPresent:   true,
				},
			},
		},
		{
			testName: "Config with no name",
			config: `---
mappings:
- match: *\.foo
  match_type: regex
  labels:
    bar: "foo"
    `,
			configBad: true,
		},
		{
			testName: "Config with labels from glob",
			config: `---
mappings:
- match: p.*.*.c.*
  match_type: glob
  name: issue_256
  labels:
    one: $1
    two: $2
    three: $3
`,
			mappings: mappings{
				{
					statsdMetric: "p.one.two.c.three",
					name:         "issue_256",
					labels: map[string]string{
						"one":   "one",
						"two":   "two",
						"three": "three",
					},
				},
			},
		},
		{
			testName: "Example from the README",
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
				{
					statsdMetric: "test.dispatcher.FooProcessor.send.success",
					name:         "dispatcher_events_total",
					labels: map[string]string{
						"processor": "FooProcessor",
						"action":    "send",
						"outcome":   "success",
						"job":       "test_dispatcher",
					},
				},
				{
					statsdMetric: "foo_product.signup.facebook.failure",
					name:         "signup_events_total",
					labels: map[string]string{
						"provider": "facebook",
						"outcome":  "failure",
						"job":      "foo_product_server",
					},
				},
				{
					statsdMetric: "test.web-server.foo.bar",
					name:         "test_web_server_foo_bar",
					labels:       map[string]string{},
				},
			},
		},
		{
			testName: "Config that drops all",
			config: `mappings:
- match: .
  match_type: regex
  name: "drop"
  action: drop`,
			mappings: mappings{
				{
					statsdMetric: "test.a",
				},
				{
					statsdMetric: "abc",
				},
			},
		},
		{
			testName: "Config that has a catch-all to drop all",
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
				{
					statsdMetric: "test.a",
				},
				{
					statsdMetric: "web.localhost",
					name:         "web",
					labels: map[string]string{
						"site": "localhost",
					},
				},
			},
		},
		{
			testName: "Config that has a ttl",
			config: `mappings:
- match: web.*
  name: "web"
  ttl: 10s
  labels:
    site: "$1"`,
			mappings: mappings{
				{
					statsdMetric: "test.a",
				},
				{
					statsdMetric: "web.localhost",
					name:         "web",
					labels: map[string]string{
						"site": "localhost",
					},
					ttl: time.Second * 10,
				},
			},
		},
		{
			testName: "Config that has a default ttl",
			config: `defaults:
  ttl: 1m2s
mappings:
- match: web.*
  name: "web"
  labels:
    site: "$1"`,
			mappings: mappings{
				{
					statsdMetric: "test.a",
				},
				{
					statsdMetric: "web.localhost",
					name:         "web",
					labels: map[string]string{
						"site": "localhost",
					},
					ttl: time.Minute + time.Second*2,
				},
			},
		},
		{
			testName: "Config that override a default ttl",
			config: `defaults:
  ttl: 1m2s
mappings:
- match: web.*
  name: "web"
  ttl: 5s
  labels:
    site: "$1"`,
			mappings: mappings{
				{
					statsdMetric: "test.a",
				},
				{
					statsdMetric: "web.localhost",
					name:         "web",
					labels: map[string]string{
						"site": "localhost",
					},
					ttl: time.Second * 5,
				},
			},
		},
	}

	mapper := MetricMapper{}
	for i, scenario := range scenarios {
		if scenario.testName == "" {
			t.Fatalf("Missing testName in scenario %+v", scenario)
		}
		t.Run(scenario.testName, func(t *testing.T) {
			err := mapper.InitFromYAMLString(scenario.config, 1000)
			if err != nil && !scenario.configBad {
				t.Fatalf("%d. Config load error: %s %s", i, scenario.config, err)
			}
			if err == nil && scenario.configBad {
				t.Fatalf("%d. Expected bad config, but loaded ok: %s", i, scenario.config)
			}

			for metric, mapping := range scenario.mappings {
				// exporter will call mapper.GetMapping with valid MetricType
				// so we also pass a sane MetricType in testing if it's not specified
				mapType := mapping.metricType
				if mapType == "" {
					mapType = MetricTypeCounter
				}
				m, labels, present := mapper.GetMapping(mapping.statsdMetric, mapType)
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
				if mapping.ttl > 0 && mapping.ttl != m.Ttl {
					t.Fatalf("%d.%q: Expected ttl of %s, got %s", i, metric, mapping.ttl.String(), m.Ttl.String())
				}
				if mapping.metricType != "" && mapType != m.MatchMetricType {
					t.Fatalf("%d.%q: Expected match metric of %s, got %s", i, metric, mapType, m.MatchMetricType)
				}

				if len(mapping.buckets) != 0 {
					if len(mapping.buckets) != len(m.HistogramOptions.Buckets) {
						t.Fatalf("%d.%q: Expected %d buckets, got %d", i, metric, len(mapping.buckets), len(m.HistogramOptions.Buckets))
					}
					for i, bucket := range mapping.buckets {
						if bucket != m.HistogramOptions.Buckets[i] {
							t.Fatalf("%d.%q: Expected bucket %v, got %v", i, metric, m.HistogramOptions.Buckets[i], bucket)
						}
					}
				}

				if len(mapping.quantiles) != 0 {
					if len(mapping.quantiles) != len(m.SummaryOptions.Quantiles) {
						t.Fatalf("%d.%q: Expected %d quantiles, got %d", i, metric, len(mapping.quantiles), len(m.SummaryOptions.Quantiles))
					}
					for i, quantile := range mapping.quantiles {
						if quantile.Quantile != m.SummaryOptions.Quantiles[i].Quantile {
							t.Fatalf("%d.%q: Expected quantile %v, got %v", i, metric, m.SummaryOptions.Quantiles[i].Quantile, quantile.Quantile)
						}
						if quantile.Error != m.SummaryOptions.Quantiles[i].Error {
							t.Fatalf("%d.%q: Expected Error margin %v, got %v", i, metric, m.SummaryOptions.Quantiles[i].Error, quantile.Error)
						}
					}
				}
				if mapping.maxAge != 0 && mapping.maxAge != m.SummaryOptions.MaxAge {
					t.Fatalf("%d.%q: Expected max age %v, got %v", i, metric, mapping.maxAge, m.SummaryOptions.MaxAge)
				}
				if mapping.ageBuckets != 0 && mapping.ageBuckets != m.SummaryOptions.AgeBuckets {
					t.Fatalf("%d.%q: Expected max age %v, got %v", i, metric, mapping.ageBuckets, m.SummaryOptions.AgeBuckets)
				}
				if mapping.bufCap != 0 && mapping.bufCap != m.SummaryOptions.BufCap {
					t.Fatalf("%d.%q: Expected max age %v, got %v", i, metric, mapping.bufCap, m.SummaryOptions.BufCap)
				}
			}
		})
	}
}

func TestAction(t *testing.T) {
	scenarios := []struct {
		testName       string
		config         string
		configBad      bool
		expectedAction ActionType
	}{
		{
			testName: "no action set",
			config: `---
mappings:
- match: test.*.*
  name: "foo"
`,
			configBad:      false,
			expectedAction: ActionTypeMap,
		},
		{
			testName: "map action set",
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
			testName: "drop action set",
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
			testName: "invalid action set",
			config: `---
mappings:
- match: test.*.*
  name: "foo"
  action: xyz
`,
			configBad:      true,
			expectedAction: ActionTypeDrop,
		},
		{
			testName: "valid yaml example",
			config: `---
mappings:
- match: "test\\.(\\w+)\\.(\\w+)\\.counter"
  match_type: regex
  name: "${2}_total"
  labels:
    provider: "$1"
`,
			configBad:      false,
			expectedAction: ActionTypeMap,
		},
		{
			testName: "invalid yaml example",
			config: `---
mappings:
- match: "test\.(\w+)\.(\w+)\.counter"
  match_type: regex
  name: "${2}_total"
  labels:
    provider: "$1"
`,
			configBad: true,
		},
	}

	for i, scenario := range scenarios {
		if scenario.testName == "" {
			t.Fatalf("Missing testName in scenario %+v", scenario)
		}
		t.Run(scenario.testName, func(t *testing.T) {
			mapper := MetricMapper{}
			err := mapper.InitFromYAMLString(scenario.config, 0)
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
		})
	}
}

// Test for https://github.com/prometheus/statsd_exporter/issues/273
// Corrupt cache for multiple names matching in fsm
func TestMultipleMatches(t *testing.T) {
	config := `---
mappings:
- match: aa.bb.*.*
  name: "aa_bb_${1}_total"
  labels:
  app: "$2"
`
	mapper := MetricMapper{}
	err := mapper.InitFromYAMLString(config, 0)
	if err != nil {
		t.Fatalf("config load error: %s ", err)
	}

	names := map[string]string{
		"aa.bb.aa.myapp": "aa_bb_aa_total",
		"aa.bb.bb.myapp": "aa_bb_bb_total",
		"aa.bb.cc.myapp": "aa_bb_cc_total",
		"aa.bb.dd.myapp": "aa_bb_dd_total",
	}

	scenarios := []struct {
		cacheSize int
	}{
		{
			cacheSize: 0,
		},
		{
			cacheSize: len(names),
		},
	}

	for i, scenario := range scenarios {
		mapper.InitCache(scenario.cacheSize)

		// run multiple times to ensure cache works as expected
		for j := 0; j < 10; j++ {
			for name, expected := range names {
				m, _, ok := mapper.GetMapping(name, MetricTypeCounter)
				if !ok {
					t.Fatalf("%d:%d Did not find match for %s", i, j, name)
				}
				if m.Name != expected {
					t.Fatalf("%d:%d Expected name %s, got %s", i, j, expected, m.Name)
				}
			}
		}
	}

}
