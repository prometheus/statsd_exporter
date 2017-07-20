// Copyright 2014 The Prometheus Authors
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

// TODO: split into separate package

package main

import (
	"bytes"
	"fmt"
	"sort"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"

	dto "github.com/prometheus/client_model/go"
)

const (
	// Capacity for the channel to collect metrics and descriptors.
	capMetricChan = 1000
	capDescChan   = 10
)

var (
	registry = NewMetricsRegistry()
)

// For statsd all our metrics will be both a Metric and a collector -- to make life easier
// we'll define an interface
type CollectorMetric interface {
	prometheus.Metric
	prometheus.Collector
}

// TODO: remove once https://github.com/prometheus/client_golang/pull/326 is merged
type Metric struct {
	// TODO: remove once these have getters upstream
	Name string
	Help string

	M CollectorMetric
}

// TODO: change back to using the regular mechanisms to determine collisions after https://github.com/prometheus/client_golang/pull/326 is merged
// then we can actually implement the registerer interface
type MetricsRegistry struct {
	mtx           sync.RWMutex
	metricsByID   map[uint64]*Metric // ID is a hash of the descIDs.
	metricsByDesc map[*prometheus.Desc]*Metric
}

// NewRegistry creates a new vanilla Registry without any Collectors
// pre-registered.
func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		metricsByID:   map[uint64]*Metric{},
		metricsByDesc: map[*prometheus.Desc]*Metric{},
	}
}

// TODO: (deosn't work now) Register implements Registerer.
func (r *MetricsRegistry) Register(metricID uint64, m *Metric) error {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	if _, exists := r.metricsByID[metricID]; exists {
		/* TODO?
		return prometheus.AlreadyRegisteredError{
			ExistingCollector: existing,
			NewCollector:      c,
		}
		*/
		return fmt.Errorf("Duplicate metric hash")
	} else {
		r.metricsByID[metricID] = m
		r.metricsByDesc[m.M.Desc()] = m
		return nil
	}
}

// TODO: (deosn't work now) Unregister implements Registerer.
func (r *MetricsRegistry) Unregister(metricID uint64) bool {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	if existingMetric, exists := r.metricsByID[metricID]; exists {
		delete(r.metricsByID, metricID)
		delete(r.metricsByDesc, existingMetric.M.Desc())
		return true
	}
	return false
}

// Gather implements Gatherer.
func (r *MetricsRegistry) Gather() ([]*dto.MetricFamily, error) {
	var (
		metricChan = make(chan prometheus.Metric, capMetricChan)
		wg         sync.WaitGroup
		errs       MultiError // The collected errors to return in the end.
	)

	r.mtx.RLock()
	metricFamiliesByName := make(map[string]*dto.MetricFamily)

	// Scatter.
	// (Collectors could be complex and slow, so we call them all at once.)
	wg.Add(len(r.metricsByID))
	go func() {
		wg.Wait()
		close(metricChan)
	}()
	for _, metric := range r.metricsByID {
		go func(collector prometheus.Collector) {
			defer wg.Done()
			collector.Collect(metricChan)
		}(metric.M)
	}

	// TODO: better? Right now since we have to do mapping of metric values to names
	// using the Desc struct pointer (pretty nasty hack -- but works). Once the PR
	// mentioned above (for adding Getter methods) is merged we can switch to using
	// that and then go back to unlocking here.
	//r.mtx.RUnlock()
	defer r.mtx.RUnlock()

	// Drain metricChan in case of premature return.
	defer func() {
		for range metricChan {
		}
	}()

	// Gather.
	for metric := range metricChan {
		// This could be done concurrently, too, but it required locking
		// of metricFamiliesByName (and of metricHashes if checks are
		// enabled). Most likely not worth it.
		desc := metric.Desc()
		m := r.metricsByDesc[desc]
		dtoMetric := &dto.Metric{}
		if err := metric.Write(dtoMetric); err != nil {
			errs = append(errs, fmt.Errorf(
				"error collecting metric %v: %s", desc, err,
			))
			continue
		}
		metricFamily, ok := metricFamiliesByName[m.Name]
		if ok {
			// TODO(beorn7): Simplify switch once Desc has type.
			switch metricFamily.GetType() {
			case dto.MetricType_COUNTER:
				if dtoMetric.Counter == nil {
					errs = append(errs, fmt.Errorf(
						"collected metric %s %s should be a Counter",
						desc.String(), dtoMetric,
					))
					continue
				}
			case dto.MetricType_GAUGE:
				if dtoMetric.Gauge == nil {
					errs = append(errs, fmt.Errorf(
						"collected metric %s %s should be a Gauge",
						desc.String(), dtoMetric,
					))
					continue
				}
			case dto.MetricType_SUMMARY:
				if dtoMetric.Summary == nil {
					errs = append(errs, fmt.Errorf(
						"collected metric %s %s should be a Summary",
						desc.String(), dtoMetric,
					))
					continue
				}
			case dto.MetricType_UNTYPED:
				if dtoMetric.Untyped == nil {
					errs = append(errs, fmt.Errorf(
						"collected metric %s %s should be Untyped",
						desc.String(), dtoMetric,
					))
					continue
				}
			case dto.MetricType_HISTOGRAM:
				if dtoMetric.Histogram == nil {
					errs = append(errs, fmt.Errorf(
						"collected metric %s %s should be a Histogram",
						desc.String(), dtoMetric,
					))
					continue
				}
			default:
				panic("encountered MetricFamily with invalid type")
			}
		} else {
			metricFamily = &dto.MetricFamily{}
			// TODO?
			metricFamily.Name = proto.String(m.Name)
			metricFamily.Help = proto.String(m.Help)
			// TODO(beorn7): Simplify switch once Desc has type.
			switch {
			case dtoMetric.Gauge != nil:
				metricFamily.Type = dto.MetricType_GAUGE.Enum()
			case dtoMetric.Counter != nil:
				metricFamily.Type = dto.MetricType_COUNTER.Enum()
			case dtoMetric.Summary != nil:
				metricFamily.Type = dto.MetricType_SUMMARY.Enum()
			case dtoMetric.Untyped != nil:
				metricFamily.Type = dto.MetricType_UNTYPED.Enum()
			case dtoMetric.Histogram != nil:
				metricFamily.Type = dto.MetricType_HISTOGRAM.Enum()
			default:
				errs = append(errs, fmt.Errorf(
					"empty metric collected: %s", dtoMetric,
				))
				continue
			}
			metricFamiliesByName[desc.String()] = metricFamily
		}
		metricFamily.Metric = append(metricFamily.Metric, dtoMetric)
	}
	return normalizeMetricFamilies(metricFamiliesByName), errs.MaybeUnwrap()
}

// TODO: update to latest client??
// MultiError is a slice of errors implementing the error interface. It is used
// by a Gatherer to report multiple errors during MetricFamily gathering.
type MultiError []error

func (errs MultiError) Error() string {
	if len(errs) == 0 {
		return ""
	}
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%d error(s) occurred:", len(errs))
	for _, err := range errs {
		fmt.Fprintf(buf, "\n* %s", err)
	}
	return buf.String()
}

// MaybeUnwrap returns nil if len(errs) is 0. It returns the first and only
// contained error as error if len(errs is 1). In all other cases, it returns
// the MultiError directly. This is helpful for returning a MultiError in a way
// that only uses the MultiError if needed.
func (errs MultiError) MaybeUnwrap() error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return errs
	}
}

// normalizeMetricFamilies returns a MetricFamily slice with empty
// MetricFamilies pruned and the remaining MetricFamilies sorted by name within
// the slice, with the contained Metrics sorted within each MetricFamily.
func normalizeMetricFamilies(metricFamiliesByName map[string]*dto.MetricFamily) []*dto.MetricFamily {
	for _, mf := range metricFamiliesByName {
		sort.Sort(metricSorter(mf.Metric))
	}
	names := make([]string, 0, len(metricFamiliesByName))
	for name, mf := range metricFamiliesByName {
		if len(mf.Metric) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]*dto.MetricFamily, 0, len(names))
	for _, name := range names {
		result = append(result, metricFamiliesByName[name])
	}
	return result
}

// metricSorter is a sortable slice of *dto.Metric.
type metricSorter []*dto.Metric

func (s metricSorter) Len() int {
	return len(s)
}

func (s metricSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s metricSorter) Less(i, j int) bool {
	if len(s[i].Label) != len(s[j].Label) {
		// This should not happen. The metrics are
		// inconsistent. However, we have to deal with the fact, as
		// people might use custom collectors or metric family injection
		// to create inconsistent metrics. So let's simply compare the
		// number of labels in this case. That will still yield
		// reproducible sorting.
		return len(s[i].Label) < len(s[j].Label)
	}
	for n, lp := range s[i].Label {
		vi := lp.GetValue()
		vj := s[j].Label[n].GetValue()
		if vi != vj {
			return vi < vj
		}
	}

	// We should never arrive here. Multiple metrics with the same
	// label set in the same scrape will lead to undefined ingestion
	// behavior. However, as above, we have to provide stable sorting
	// here, even for inconsistent metrics. So sort equal metrics
	// by their timestamp, with missing timestamps (implying "now")
	// coming last.
	if s[i].TimestampMs == nil {
		return false
	}
	if s[j].TimestampMs == nil {
		return true
	}
	return s[i].GetTimestampMs() < s[j].GetTimestampMs()
}
