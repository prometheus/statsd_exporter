// Copyright 2022 The Prometheus Authors
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

package relay

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/stvp/go-udp-testing"
)

func TestRelay_RelayLine(t *testing.T) {
	type args struct {
		lines    []string
		expected string
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "multiple lines",
			args: args{
				lines:    []string{"foo5:100|c|#tag1:bar,#tag2:baz", "foo2:200|c|#tag1:bar,#tag2:baz"},
				expected: "foo5:100|c|#tag1:bar,#tag2:baz\n",
			},
		},
	}

	const testAddr = "[::1]:1160"

	for _, tt := range tests {
		udp.SetAddr(testAddr)
		t.Run(tt.name, func(t *testing.T) {
			tickerCh := make(chan time.Time)
			clock.ClockInstance = &clock.Clock{
				TickerCh: tickerCh,
			}
			clock.ClockInstance.Instant = time.Unix(0, 0)

			logger := log.NewNopLogger()
			r, err := NewRelay(
				logger,
				testAddr,
				200,
			)

			if err != nil {
				t.Errorf("Did not expect error while creating relay.")
			}

			udp.ShouldReceive(t, tt.args.expected, func() {
				for _, line := range tt.args.lines {
					r.RelayLine(line)
				}

				for goSchedTimes := 0; goSchedTimes < 1000; goSchedTimes++ {
					if len(r.bufferChannel) == 0 {
						break
					}
					runtime.Gosched()
				}

				// Tick time forward to trigger a packet send.
				clock.ClockInstance.Instant = time.Unix(1, 10)
				clock.ClockInstance.TickerCh <- time.Unix(0, 0)
			})

			metrics, err := prometheus.DefaultGatherer.Gather()
			if err != nil {
				t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
			}

			metricNames := map[string]float64{
				"statsd_exporter_relay_long_lines_total":    0,
				"statsd_exporter_relay_lines_relayed_total": float64(len(tt.args.lines)),
			}
			for metricName, expectedValue := range metricNames {
				metric := getFloat64(metrics, metricName, prometheus.Labels{"target": testAddr})

				if metric == nil {
					t.Fatalf("Could not find time series with first label set for metric: %s", metricName)
				}
				if *metric != expectedValue {
					t.Errorf("Expected metric %s to be %f, got %f", metricName, expectedValue, *metric)
				}
			}

			prometheus.Unregister(relayLongLinesTotal)
			prometheus.Unregister(relayLinesRelayedTotal)
		})
	}
}

// getFloat64 search for metric by name in array of MetricFamily and then search a value by labels.
// Method returns a value or nil if metric is not found.
func getFloat64(metrics []*dto.MetricFamily, name string, labels prometheus.Labels) *float64 {
	var metricFamily *dto.MetricFamily
	for _, m := range metrics {
		if *m.Name == name {
			metricFamily = m
			break
		}
	}
	if metricFamily == nil {
		return nil
	}

	var metric *dto.Metric
	labelStr := fmt.Sprintf("%v", labels)
	for _, m := range metricFamily.Metric {
		l := labelPairsAsLabels(m.GetLabel())
		ls := fmt.Sprintf("%v", l)
		if labelStr == ls {
			metric = m
			break
		}
	}
	if metric == nil {
		return nil
	}

	var value float64
	if metric.Gauge != nil {
		value = metric.Gauge.GetValue()
		return &value
	}
	if metric.Counter != nil {
		value = metric.Counter.GetValue()
		return &value
	}
	if metric.Histogram != nil {
		value = metric.Histogram.GetSampleSum()
		return &value
	}
	if metric.Summary != nil {
		value = metric.Summary.GetSampleSum()
		return &value
	}
	if metric.Untyped != nil {
		value = metric.Untyped.GetValue()
		return &value
	}
	panic(fmt.Errorf("collected a non-gauge/counter/histogram/summary/untyped metric: %s", metric))
}

func labelPairsAsLabels(pairs []*dto.LabelPair) (labels prometheus.Labels) {
	labels = prometheus.Labels{}
	for _, pair := range pairs {
		if pair.Name == nil {
			continue
		}
		value := ""
		if pair.Value != nil {
			value = *pair.Value
		}
		labels[*pair.Name] = value
	}
	return
}
