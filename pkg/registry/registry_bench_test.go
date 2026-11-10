// Copyright The Prometheus Authors
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

package registry

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

// clearMetricsPerSeries is the previous ClearMetrics implementation, which
// deletes every series individually. It is kept as the benchmark baseline.
func clearMetricsPerSeries(r *Registry) int {
	removed := 0
	for _, metric := range r.Metrics {
		for hash, rm := range metric.Metrics {
			r.removeMetric(metric, hash, rm)
			removed++
		}
	}
	return removed
}

func populateGauges(b *testing.B, r *Registry, metricsCount *prometheus.GaugeVec, names, seriesPerName int) {
	b.Helper()
	mapping := &mapper.MetricMapping{}
	for n := 0; n < names; n++ {
		name := "bench_gauge_" + strconv.Itoa(n)
		for s := 0; s < seriesPerName; s++ {
			g, err := r.GetGauge(name, prometheus.Labels{"series": strconv.Itoa(s)}, "help", mapping, metricsCount)
			if err != nil {
				b.Fatal(err)
			}
			g.Set(1)
		}
	}
}

// BenchmarkClearMetrics compares per-series deletes with one Reset per vector.
// In the "scraping" variants a goroutine starts scraping the registry in a loop
// just before each clear, and the longest scrape is reported as max-scrape-ns.
func BenchmarkClearMetrics(b *testing.B) {
	shapes := []struct {
		name                 string
		names, seriesPerName int
	}{
		{"one_vector_100000_series", 1, 100000},
		{"10000_names_10_series", 10000, 10},
	}
	clears := []struct {
		name  string
		clear func(*Registry) int
	}{
		{"per_series_delete", clearMetricsPerSeries},
		{"reset", (*Registry).ClearMetrics},
	}
	for _, shape := range shapes {
		for _, c := range clears {
			for _, scrape := range []bool{false, true} {
				benchmarkClear(b, shape.name, shape.names, shape.seriesPerName, c.name, c.clear, scrape)
			}
		}
	}
}

func benchmarkClear(b *testing.B, shape string, names, seriesPerName int, clearName string, clearFn func(*Registry) int, scrape bool) {
	name := shape + "/" + clearName + "/idle"
	if scrape {
		name = shape + "/" + clearName + "/scraping"
	}
	b.Run(name, func(b *testing.B) {
		var maxScrape time.Duration
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			r, reg, metricsCount := newTestRegistry()
			populateGauges(b, r, metricsCount, names, seriesPerName)
			stop := make(chan struct{})
			var wg sync.WaitGroup
			if scrape {
				started := make(chan struct{})
				wg.Add(1)
				go func() {
					defer wg.Done()
					for first := true; ; first = false {
						select {
						case <-stop:
							return
						default:
						}
						if first {
							close(started)
						}
						start := time.Now()
						if _, err := reg.Gather(); err != nil {
							b.Error(err)
							return
						}
						maxScrape = max(maxScrape, time.Since(start))
					}
				}()
				// Start clearing while a scrape is in flight.
				<-started
			}
			b.StartTimer()
			if got, want := clearFn(r), names*seriesPerName; got != want {
				b.Fatalf("expected to clear %d series, cleared %d", want, got)
			}
			b.StopTimer()
			close(stop)
			wg.Wait()
		}
		if scrape {
			b.ReportMetric(float64(maxScrape.Nanoseconds()), "max-scrape-ns")
		}
	})
}
