package relay

import (
	"fmt"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/stvp/go-udp-testing"
	"testing"
	"time"
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
			name: "single line",
			args: args{
				lines:    []string{"foo5:100|c|#tag1:bar,#tag2:baz"},
				expected: "foo5:100|c|#tag1:bar,#tag2:baz\n",
			},
		},
	}

	for _, tt := range tests {
		udp.SetAddr(":1160")
		t.Run(tt.name, func(t *testing.T) {

			tickerCh := make(chan time.Time)
			clock.ClockInstance = &clock.Clock{
				TickerCh: tickerCh,
			}
			clock.ClockInstance.Instant = time.Unix(0, 0)

			logger := log.NewNopLogger()
			r, err := NewRelay(
				logger,
				"localhost:1160",
				200,
			)

			if err != nil {
				t.Errorf("Did not expect error while creating relay.")
			}

			udp.ShouldReceive(t, tt.args.expected, func() {
				for _, line := range tt.args.lines {
					r.RelayLine(line)
				}
				// Tick time forward to trigger a packet send.
				clock.ClockInstance.Instant = time.Unix(20000, 0)
				clock.ClockInstance.TickerCh <- time.Unix(20000, 0)
			})

			metrics, err := prometheus.DefaultGatherer.Gather()
			if err != nil {
				t.Fatalf("Cannot gather from DefaultGatherer: %v", err)
			}

			metricNames := map[string]float64{
				"statsd_exporter_relay_long_lines_total": 0,
			}
			for metricName, expectedValue := range metricNames {
				firstMetric := getFloat64(metrics, metricName, prometheus.Labels{"target": "localhost:1160"})

				if firstMetric == nil {
					t.Fatalf("Could not find time series with first label set for metric: %s", metricName)
				}
				if *firstMetric != expectedValue {
					t.Errorf("Expected metric %s to be %f, got %f", metricName, expectedValue, *firstMetric)
				}
			}
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
