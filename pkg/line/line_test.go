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

	"github.com/go-kit/kit/log"
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
		"timings with sampling factor": {
			in: "foo.timing:0.5|ms|@0.1",
			out: event.Events{
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
				&event.ObserverEvent{OMetricName: "foo.timing", OValue: 0.0005, OLabels: map[string]string{}},
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
	}

	DogstatsdTagsEnabled = true
	InfluxdbTagsEnabled = true
	SignalFXTagsEnabled = true
	LibratoTagsEnabled = true

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

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

	DogstatsdTagsEnabled = false
	InfluxdbTagsEnabled = false
	SignalFXTagsEnabled = false
	LibratoTagsEnabled = false

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

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

	DogstatsdTagsEnabled = false
	InfluxdbTagsEnabled = true
	SignalFXTagsEnabled = true
	LibratoTagsEnabled = true

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

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

	DogstatsdTagsEnabled = true
	InfluxdbTagsEnabled = false
	SignalFXTagsEnabled = true
	LibratoTagsEnabled = true

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

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

	DogstatsdTagsEnabled = true
	InfluxdbTagsEnabled = true
	SignalFXTagsEnabled = false
	LibratoTagsEnabled = true

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

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

	DogstatsdTagsEnabled = true
	InfluxdbTagsEnabled = true
	SignalFXTagsEnabled = true
	LibratoTagsEnabled = false

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			events := LineToEvents(testCase.in, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, nopLogger)

			for j, expected := range testCase.out {
				if !reflect.DeepEqual(&expected, &events[j]) {
					t.Fatalf("Expected %#v, got %#v in scenario '%s'", expected, events[j], name)
				}
			}
		})
	}
}
