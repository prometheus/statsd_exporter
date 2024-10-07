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

package line

import (
	"reflect"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/statsd_exporter/pkg/event"
)

var (
	nopSamplesReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_samples_total",
			Help: "The total number of StatsD samples received.",
		},
	)
	nopSampleErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_sample_errors_total",
			Help: "The total number of errors parsing StatsD samples.",
		},
		[]string{"reason"},
	)
	nopTagsReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tags_total",
			Help: "The total number of DogStatsD tags processed.",
		},
	)
	nopTagErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tag_errors_total",
			Help: "The number of errors parsing DogStatsD tags.",
		},
	)
	nopLogger = log.NewNopLogger()
)

func TestLineToEvents(t *testing.T) {
	type testCase struct {
		in  string
		out event.Events
	}

	testCases := map[string]testCase{
		"empty": {},
		"simple counter": {
			in: "foo:2|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      2,
					CLabels:     map[string]string{},
				},
			},
		},
		"simple gauge": {
			in: "foo:3|g",
			out: event.Events{
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      3,
					GLabels:     map[string]string{},
				},
			},
		},
		"gauge with sampling": {
			in: "foo:3|g|@0.2",
			out: event.Events{
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      3,
					GLabels:     map[string]string{},
				},
			},
		},
		"gauge decrement": {
			in: "foo:-10|g",
			out: event.Events{
				&event.GaugeEvent{
					GMetricName: "foo",
					GValue:      -10,
					GRelative:   true,
					GLabels:     map[string]string{},
				},
			},
		},
		"simple timer": {
			in: "foo:200|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.2,
					OLabels:     map[string]string{},
				},
			},
		},
		"simple histogram": {
			in: "foo:200|h",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		},
		"simple distribution": {
			in: "foo:200|d",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		},
		"distribution with sampling": {
			in: "foo:0.01|d|@0.2|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"librato tag extension": {
			in: "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"librato tag extension with tag keys unsupported by prometheus": {
			in: "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"influxdb tag extension": {
			in: "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension": {
			in: "foo.[tag1=bar,tag2=baz]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at end of name": {
			in: "foo.test[tag1=bar,tag2=baz]:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at beginning of name": {
			in: "[tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, no tags": {
			in: "foo.[]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, non-kv tags": {
			in: "foo.[tag1,tag2]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing closing bracket": {
			in: "[tag1=bar,tag2=bazfoo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar,tag2=bazfoo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing opening bracket": {
			in: "tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "tag1=bar,tag2=baz]foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"influxdb tag extension with tag keys unsupported by prometheus": {
			in: "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension": {
			in: "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with # in all keys (as sent by datadog php client)": {
			in: "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with tag keys unsupported by prometheus": {
			in: "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension with valueless tags: ignored": {
			in: "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags (edge case)": {
			in: "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with empty tags (edge case)": {
			in: "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with sampling": {
			in: "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"librato/dogstatsd mixed tag styles without sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"signalfx/dogstatsd mixed tag styles without sampling": {
			in:  "foo[tag1=foo,tag3=bing]:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"influxdb/dogstatsd mixed tag styles without sampling": {
			in:  "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"mixed tag styles with sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"histogram with sampling": {
			in: "foo:0.01|h|@0.2|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with multiple colons": {
			in: "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "foo:bar"},
				},
			},
		},
		"datadog tag extension with invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		},
		"datadog tag extension with both valid and invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		},
		"datadog timings with extended aggregation values": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values and sampling but without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values, sampling, and tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog histogram with extended aggregation values and tags": {
			in: "foo_histogram:0.5:120:3000:10:20000:0.01|h|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog distribution with extended aggregation values": {
			in: "foo_distribution:0.5:120:3000:10:20000:0.01|d|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog counter with invalid extended aggregation values": {
			in: "foo_counter:0.5:120:3000:10:20000:0.01|c|#tag1:bar,tag2:baz",
		},
		"datadog gauge with invalid extended aggregation values": {
			in: "foo_gauge:0.5:120:3000:10:20000:0.01|g|#tag1:bar,tag2:baz",
		},
		"datadog timing with extended aggregation values and invalid signalfx tags": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"SignalFX counter with invalid Datadog style extended aggregation values": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags counter with invalid Datadog style extended aggregation values": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and histogram type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"timings with sampling factor": {
			in: "foo.timing:0.5|ms|@0.1",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo.timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
			},
		},
		"bad line": {
			in: "foo",
		},
		"bad component": {
			in: "foo:1",
		},
		"bad value": {
			in: "foo:1o|c",
		},
		"illegal sampling factor": {
			in: "foo:1|c|@bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1,
					CLabels:     map[string]string{},
				},
			},
		},
		"zero sampling factor": {
			in: "foo:2|c|@0",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      2,
					CLabels:     map[string]string{},
				},
			},
		},
		"illegal stat type": {
			in: "foo:2|t",
		},
		"empty metric name": {
			in: ":100|ms",
		},
		"empty component": {
			in: "foo:1|c|",
		},
		"invalid utf8": {
			in: "invalid\xc3\x28utf8:1|c",
		},
		"ms timer with conversion to seconds": {
			in: "foo:200|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      0.2,
					OLabels:     map[string]string{},
				},
			},
		},
		"histogram with no unit conversion": {
			in: "foo:200|h",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		},
		"distribution with no unit conversion": {
			in: "foo:200|d",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo",
					OValue:      200,
					OLabels:     map[string]string{},
				},
			},
		},
		"invalid event split over lines part 1": {
			in: "karafka.consumer.consume.cpu_idle_second:  0.111090  -0.055903  -0.195390 (  2.419002)",
		},
		"invalid event split over lines part 2": {
			in: "|h|#consumer:Kafka::SharedConfigurationConsumer,topic:shared_configuration_update,partition:1,consumer_group:tc_rc_us",
		},
	}

	parser := NewParser()
	parser.EnableDogstatsdParsing()
	parser.EnableInfluxdbParsing()
	parser.EnableLibratoParsing()
	parser.EnableSignalFXParsing()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := parser.LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}

func TestDisableParsingLineToEvents(t *testing.T) {
	type testCase struct {
		in  string
		out event.Events
	}

	testCases := map[string]testCase{
		"librato tag extension": {
			in: "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo#tag1=bar,tag2=baz",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"librato tag extension with tag keys unsupported by prometheus": {
			in: "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo#09digits=0,tag.with.dots=1",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"influxdb tag extension": {
			in: "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo,tag1=bar,tag2=baz",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension": {
			in: "foo.[tag1=bar,tag2=baz]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.[tag1=bar,tag2=baz]test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, tags at end of name": {
			in: "foo.test[tag1=bar,tag2=baz]:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test[tag1=bar,tag2=baz]",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, tags at beginning of name": {
			in: "[tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar,tag2=baz]foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, no tags": {
			in: "foo.[]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.[]test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, non-kv tags": {
			in: "foo.[tag1,tag2]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.[tag1,tag2]test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing closing bracket": {
			in: "[tag1=bar,tag2=bazfoo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar,tag2=bazfoo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing opening bracket": {
			in: "tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "tag1=bar,tag2=baz]foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"influxdb tag extension with tag keys unsupported by prometheus": {
			in: "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo,09digits=0,tag.with.dots=1",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension": {
			in: "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with # in all keys (as sent by datadog php client)": {
			in: "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with tag keys unsupported by prometheus": {
			in: "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags: ignored": {
			in: "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags (edge case)": {
			in: "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with empty tags (edge case)": {
			in: "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with sampling": {
			in: "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values and sampling but without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values, sampling, and tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog histogram with extended aggregation values and tags": {
			in: "foo_histogram:0.5:120:3000:10:20000:0.01|h|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.5,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      3000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      10,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      20000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog distribution with extended aggregation values": {
			in: "foo_distribution:0.5:120:3000:10:20000:0.01|d|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.5,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      3000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      10,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      20000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog counter with invalid extended aggregation values": {
			in: "foo_counter:0.5:120:3000:10:20000:0.01|c|#tag1:bar,tag2:baz",
		},
		"datadog gauge with invalid extended aggregation values": {
			in: "foo_gauge:0.5:120:3000:10:20000:0.01|g|#tag1:bar,tag2:baz",
		},
		"datadog timing with extended aggregation values and invalid signalfx tags": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"SignalFX counter with invalid Datadog style extended aggregation values": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags counter with invalid Datadog style extended aggregation values": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and histogram type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"librato/dogstatsd mixed tag styles without sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"signalfx/dogstatsd mixed tag styles without sampling": {
			in:  "foo[tag1=foo,tag3=bing]:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"influxdb/dogstatsd mixed tag styles without sampling": {
			in:  "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"mixed tag styles with sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"datadog tag extension with multiple colons": {
			in: "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		},
		"datadog tag extension with both valid and invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		},
	}

	parser := NewParser()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := parser.LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}

func TestDisableParsingDogstatsdLineToEvents(t *testing.T) {
	type testCase struct {
		in  string
		out event.Events
	}

	testCases := map[string]testCase{
		"librato tag extension": {
			in: "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"librato tag extension with tag keys unsupported by prometheus": {
			in: "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"influxdb tag extension": {
			in: "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension": {
			in: "foo.[tag1=bar,tag2=baz]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at end of name": {
			in: "foo.test[tag1=bar,tag2=baz]:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at beginning of name": {
			in: "[tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, no tags": {
			in: "foo.[]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, non-kv tags": {
			in: "foo.[tag1,tag2]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing closing bracket": {
			in: "[tag1=bar,tag2=bazfoo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar,tag2=bazfoo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing opening bracket": {
			in: "tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "tag1=bar,tag2=baz]foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"influxdb tag extension with tag keys unsupported by prometheus": {
			in: "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension": {
			in: "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with # in all keys (as sent by datadog php client)": {
			in: "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with tag keys unsupported by prometheus": {
			in: "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags: ignored": {
			in: "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags (edge case)": {
			in: "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with empty tags (edge case)": {
			in: "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with sampling": {
			in: "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values and sampling but without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values, sampling, and tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog histogram with extended aggregation values and tags": {
			in: "foo_histogram:0.5:120:3000:10:20000:0.01|h|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.5,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      3000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      10,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      20000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog distribution with extended aggregation values": {
			in: "foo_distribution:0.5:120:3000:10:20000:0.01|d|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.5,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      3000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      10,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      20000,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog counter with invalid extended aggregation values": {
			in: "foo_counter:0.5:120:3000:10:20000:0.01|c|#tag1:bar,tag2:baz",
		},
		"datadog gauge with invalid extended aggregation values": {
			in: "foo_gauge:0.5:120:3000:10:20000:0.01|g|#tag1:bar,tag2:baz",
		},
		"datadog timing with extended aggregation values and invalid signalfx tags": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"SignalFX counter with invalid Datadog style extended aggregation values": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags counter with invalid Datadog style extended aggregation values": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and histogram type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"librato/dogstatsd mixed tag styles without sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"signalfx/dogstatsd mixed tag styles without sampling": {
			in:  "foo[tag1=foo,tag3=bing]:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"influxdb/dogstatsd mixed tag styles without sampling": {
			in:  "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"mixed tag styles with sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"datadog tag extension with multiple colons": {
			in: "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		},
		"datadog tag extension with both valid and invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		},
	}

	parser := NewParser()
	parser.EnableInfluxdbParsing()
	parser.EnableLibratoParsing()
	parser.EnableSignalFXParsing()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := parser.LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}

func TestDisableParsingInfluxdbLineToEvents(t *testing.T) {
	type testCase struct {
		in  string
		out event.Events
	}

	testCases := map[string]testCase{
		"librato tag extension": {
			in: "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"librato tag extension with tag keys unsupported by prometheus": {
			in: "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"influxdb tag extension": {
			in: "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo,tag1=bar,tag2=baz",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension": {
			in: "foo.[tag1=bar,tag2=baz]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at end of name": {
			in: "foo.test[tag1=bar,tag2=baz]:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at beginning of name": {
			in: "[tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, no tags": {
			in: "foo.[]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, non-kv tags": {
			in: "foo.[tag1,tag2]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing closing bracket": {
			in: "[tag1=bar,tag2=bazfoo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar,tag2=bazfoo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing opening bracket": {
			in: "tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "tag1=bar,tag2=baz]foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"influxdb tag extension with tag keys unsupported by prometheus": {
			in: "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo,09digits=0,tag.with.dots=1",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension": {
			in: "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with # in all keys (as sent by datadog php client)": {
			in: "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with tag keys unsupported by prometheus": {
			in: "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension with valueless tags: ignored": {
			in: "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags (edge case)": {
			in: "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with empty tags (edge case)": {
			in: "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with sampling": {
			in: "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values and sampling but without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values, sampling, and tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog histogram with extended aggregation values and tags": {
			in: "foo_histogram:0.5:120:3000:10:20000:0.01|h|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog distribution with extended aggregation values": {
			in: "foo_distribution:0.5:120:3000:10:20000:0.01|d|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog counter with invalid extended aggregation values": {
			in: "foo_counter:0.5:120:3000:10:20000:0.01|c|#tag1:bar,tag2:baz",
		},
		"datadog gauge with invalid extended aggregation values": {
			in: "foo_gauge:0.5:120:3000:10:20000:0.01|g|#tag1:bar,tag2:baz",
		},
		"datadog timing with extended aggregation values and invalid signalfx tags": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"SignalFX counter with invalid Datadog style extended aggregation values": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags counter with invalid Datadog style extended aggregation values": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and histogram type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"librato/dogstatsd mixed tag styles without sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"signalfx/dogstatsd mixed tag styles without sampling": {
			in:  "foo[tag1=foo,tag3=bing]:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"influxdb/dogstatsd mixed tag styles without sampling": {
			in:  "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"mixed tag styles with sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"datadog tag extension with multiple colons": {
			in: "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "foo:bar"},
				},
			},
		},
		"datadog tag extension with invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		},
		"datadog tag extension with both valid and invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		},
	}

	parser := NewParser()
	parser.EnableDogstatsdParsing()
	parser.EnableLibratoParsing()
	parser.EnableSignalFXParsing()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := parser.LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}

func TestDisableParsingSignalfxLineToEvents(t *testing.T) {
	type testCase struct {
		in  string
		out event.Events
	}

	testCases := map[string]testCase{
		"librato tag extension": {
			in: "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"librato tag extension with tag keys unsupported by prometheus": {
			in: "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"influxdb tag extension": {
			in: "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension": { // parsed as influxdb tags
			in: "foo.[tag1=bar,tag2=baz]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.[tag1=bar",
					CValue:      100,
					CLabels:     map[string]string{"tag2": "baz]test"},
				},
			},
		},
		"SignalFx tag extension, tags at end of name": { // parsed as influxdb tags
			in: "foo.test[tag1=bar,tag2=baz]:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test[tag1=bar",
					CValue:      100,
					CLabels:     map[string]string{"tag2": "baz]"},
				},
			},
		},
		"SignalFx tag extension, tags at beginning of name": { // parsed as influxdb tags
			in: "[tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar",
					CValue:      100,
					CLabels:     map[string]string{"tag2": "baz]foo.test"},
				},
			},
		},
		"SignalFx tag extension, no tags": {
			in: "foo.[]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.[]test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, non-kv tags": { // parsed as influxdb tags
			in: "foo.[tag1,tag2]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.[tag1",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing closing bracket": { // parsed as influxdb tags
			in: "[tag1=bar,tag2=bazfoo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar",
					CValue:      100,
					CLabels:     map[string]string{"tag2": "bazfoo.test"},
				},
			},
		},
		"SignalFx tag extension, missing opening bracket": { // parsed as influxdb tags
			in: "tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "tag1=bar",
					CValue:      100,
					CLabels:     map[string]string{"tag2": "baz]foo.test"},
				},
			},
		},
		"influxdb tag extension with tag keys unsupported by prometheus": {
			in: "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension": {
			in: "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with # in all keys (as sent by datadog php client)": {
			in: "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with tag keys unsupported by prometheus": {
			in: "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension with valueless tags: ignored": {
			in: "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags (edge case)": {
			in: "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with empty tags (edge case)": {
			in: "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with sampling": {
			in: "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values and sampling but without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values, sampling, and tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog histogram with extended aggregation values and tags": {
			in: "foo_histogram:0.5:120:3000:10:20000:0.01|h|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog distribution with extended aggregation values": {
			in: "foo_distribution:0.5:120:3000:10:20000:0.01|d|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog counter with invalid extended aggregation values": {
			in: "foo_counter:0.5:120:3000:10:20000:0.01|c|#tag1:bar,tag2:baz",
		},
		"datadog gauge with invalid extended aggregation values": {
			in: "foo_gauge:0.5:120:3000:10:20000:0.01|g|#tag1:bar,tag2:baz",
		},
		"datadog timing with extended aggregation values and invalid signalfx tags": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"SignalFX counter with invalid Datadog style extended aggregation values": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags counter with invalid Datadog style extended aggregation values": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and histogram type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"librato/dogstatsd mixed tag styles without sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"signalfx/dogstatsd mixed tag styles without sampling": {
			in:  "foo[tag1=foo,tag3=bing]:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"influxdb/dogstatsd mixed tag styles without sampling": {
			in:  "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"mixed tag styles with sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"datadog tag extension with multiple colons": {
			in: "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "foo:bar"},
				},
			},
		},
		"datadog tag extension with invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		},
		"datadog tag extension with both valid and invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		},
	}

	parser := NewParser()
	parser.EnableDogstatsdParsing()
	parser.EnableInfluxdbParsing()
	parser.EnableLibratoParsing()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := parser.LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}

func TestDisableParsingLibratoLineToEvents(t *testing.T) {
	type testCase struct {
		in  string
		out event.Events
	}

	testCases := map[string]testCase{
		"librato tag extension": { // parsed as influxdb tags
			in: "foo#tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo#tag1=bar",
					CValue:      100,
					CLabels:     map[string]string{"tag2": "baz"},
				},
			},
		},
		"librato tag extension with tag keys unsupported by prometheus": { // parsed as influxdb tags
			in: "foo#09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo#09digits=0",
					CValue:      100,
					CLabels:     map[string]string{"tag_with_dots": "1"},
				},
			},
		},
		"influxdb tag extension": {
			in: "foo,tag1=bar,tag2=baz:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension": {
			in: "foo.[tag1=bar,tag2=baz]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at end of name": {
			in: "foo.test[tag1=bar,tag2=baz]:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, tags at beginning of name": {
			in: "[tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"SignalFx tag extension, no tags": {
			in: "foo.[]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, non-kv tags": {
			in: "foo.[tag1,tag2]test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing closing bracket": {
			in: "[tag1=bar,tag2=bazfoo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "[tag1=bar,tag2=bazfoo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"SignalFx tag extension, missing opening bracket": {
			in: "tag1=bar,tag2=baz]foo.test:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "tag1=bar,tag2=baz]foo.test",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"influxdb tag extension with tag keys unsupported by prometheus": {
			in: "foo,09digits=0,tag.with.dots=1:100|c",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension": {
			in: "foo:100|c|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with # in all keys (as sent by datadog php client)": {
			in: "foo:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog tag extension with tag keys unsupported by prometheus": {
			in: "foo:100|c|#09digits:0,tag.with.dots:1",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"_09digits": "0", "tag_with_dots": "1"},
				},
			},
		},
		"datadog tag extension with valueless tags: ignored": {
			in: "foo:100|c|#tag_without_a_value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{},
				},
			},
		},
		"datadog tag extension with valueless tags (edge case)": {
			in: "foo:100|c|#tag_without_a_value,tag:value",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with empty tags (edge case)": {
			in: "foo:100|c|#tag:value,,",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      100,
					CLabels:     map[string]string{"tag": "value"},
				},
			},
		},
		"datadog tag extension with sampling": {
			in: "foo:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog timings with extended aggregation values without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values and sampling but without tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{},
				},
			},
		},
		"datadog timings with extended aggregation values, sampling, and tags": {
			in: "foo_timing:0.5:120:3000:10:20000:0.01|ms|@0.5|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.0005,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      3,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      20,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_timing",
					OValue:      0.00001,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog histogram with extended aggregation values and tags": {
			in: "foo_histogram:0.5:120:3000:10:20000:0.01|h|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_histogram",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog distribution with extended aggregation values": {
			in: "foo_distribution:0.5:120:3000:10:20000:0.01|d|#tag1:bar,tag2:baz",
			out: event.Events{
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.5,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      120,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      3000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      10,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      20000,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
				&event.ObserverEvent{
					OMetricName: "foo_distribution",
					OValue:      0.01,
					OLabels:     map[string]string{"tag1": "bar", "tag2": "baz"},
				},
			},
		},
		"datadog counter with invalid extended aggregation values": {
			in: "foo_counter:0.5:120:3000:10:20000:0.01|c|#tag1:bar,tag2:baz",
		},
		"datadog gauge with invalid extended aggregation values": {
			in: "foo_gauge:0.5:120:3000:10:20000:0.01|g|#tag1:bar,tag2:baz",
		},
		"datadog timing with extended aggregation values and invalid signalfx tags": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"SignalFX counter with invalid Datadog style extended aggregation values": {
			in: "foo.[tag1=bar,tag2=baz]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags counter with invalid Datadog style extended aggregation values": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|c",
		},
		"SignalFX no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.[]test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and timings type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"Influx no tags with invalid Datadog style extended aggregation values and histogram type": {
			in: "foo.test:0.5:120:3000:10:20000:0.01|ms",
		},
		"librato/dogstatsd mixed tag styles without sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"signalfx/dogstatsd mixed tag styles without sampling": {
			in:  "foo[tag1=foo,tag3=bing]:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"influxdb/dogstatsd mixed tag styles without sampling": {
			in:  "foo,tag1=foo,tag3=bing:100|c|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"mixed tag styles with sampling": {
			in:  "foo#tag1=foo,tag3=bing:100|c|@0.1|#tag1:bar,#tag2:baz",
			out: event.Events{},
		},
		"datadog tag extension with multiple colons": {
			in: "foo:100|c|@0.1|#tag1:foo:bar",
			out: event.Events{
				&event.CounterEvent{
					CMetricName: "foo",
					CValue:      1000,
					CLabels:     map[string]string{"tag1": "foo:bar"},
				},
			},
		},
		"datadog tag extension with invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag:\xc3\x28invalid",
		},
		"datadog tag extension with both valid and invalid utf8 tag values": {
			in: "foo:100|c|@0.1|#tag1:valid,tag2:\xc3\x28invalid",
		},
	}

	parser := NewParser()
	parser.EnableDogstatsdParsing()
	parser.EnableInfluxdbParsing()
	parser.EnableSignalFXParsing()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := parser.LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}
