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

package event

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

type Event interface {
	MetricName() string
	Value() float64
	Labels() map[string]string
	MetricType() mapper.MetricType
}

type CounterEvent struct {
	CMetricName string
	CValue      float64
	CLabels     map[string]string
}

func (c *CounterEvent) MetricName() string            { return c.CMetricName }
func (c *CounterEvent) Value() float64                { return c.CValue }
func (c *CounterEvent) Labels() map[string]string     { return c.CLabels }
func (c *CounterEvent) MetricType() mapper.MetricType { return mapper.MetricTypeCounter }
func (c *CounterEvent) Values() []float64             { return []float64{c.CValue} }

type GaugeEvent struct {
	GMetricName string
	GValue      float64
	GRelative   bool
	GLabels     map[string]string
}

func (g *GaugeEvent) MetricName() string            { return g.GMetricName }
func (g *GaugeEvent) Value() float64                { return g.GValue }
func (g *GaugeEvent) Labels() map[string]string     { return g.GLabels }
func (g *GaugeEvent) MetricType() mapper.MetricType { return mapper.MetricTypeGauge }
func (g *GaugeEvent) Values() []float64             { return []float64{g.GValue} }

type ObserverEvent struct {
	OMetricName string
	OValue      float64
	OLabels     map[string]string
}

func (o *ObserverEvent) MetricName() string            { return o.OMetricName }
func (o *ObserverEvent) Value() float64                { return o.OValue }
func (o *ObserverEvent) Labels() map[string]string     { return o.OLabels }
func (o *ObserverEvent) MetricType() mapper.MetricType { return mapper.MetricTypeObserver }
func (o *ObserverEvent) Values() []float64             { return []float64{o.OValue} }

type Events []Event

type EventQueue struct {
	C              chan Events
	q              Events
	m              sync.Mutex
	flushTicker    *time.Ticker
	flushThreshold int
	flushInterval  time.Duration
	eventsFlushed  prometheus.Counter
}

type EventHandler interface {
	Queue(event Events)
}

func NewEventQueue(c chan Events, flushThreshold int, flushInterval time.Duration, eventsFlushed prometheus.Counter) *EventQueue {
	ticker := clock.NewTicker(flushInterval)
	eq := &EventQueue{
		C:              c,
		flushThreshold: flushThreshold,
		flushInterval:  flushInterval,
		flushTicker:    ticker,
		q:              make([]Event, 0, flushThreshold),
		eventsFlushed:  eventsFlushed,
	}
	go func() {
		for {
			<-ticker.C
			eq.Flush()
		}
	}()
	return eq
}

func (eq *EventQueue) Queue(events Events) {
	eq.m.Lock()
	defer eq.m.Unlock()

	for _, e := range events {
		eq.q = append(eq.q, e)
		if len(eq.q) >= eq.flushThreshold {
			eq.FlushUnlocked()
		}
	}
}

func (eq *EventQueue) Flush() {
	eq.m.Lock()
	defer eq.m.Unlock()
	eq.FlushUnlocked()
}

func (eq *EventQueue) FlushUnlocked() {
	eq.C <- eq.q
	eq.q = make([]Event, 0, cap(eq.q))
	eq.eventsFlushed.Inc()
}

func (eq *EventQueue) Len() int {
	eq.m.Lock()
	defer eq.m.Unlock()

	return len(eq.q)
}

type UnbufferedEventHandler struct {
	C chan Events
}

func (ueh *UnbufferedEventHandler) Queue(events Events) {
	ueh.C <- events
}

// MultiValueEvent is an event that contains multiple values, it is going to replace the existing Event interface.
type MultiValueEvent interface {
	MetricName() string
	Labels() map[string]string
	MetricType() mapper.MetricType
	Values() []float64
}

type MultiObserverEvent struct {
	OMetricName string
	OValues     []float64 // DataDog extensions allow multiple values in a single sample
	OLabels     map[string]string
	SampleRate  float64
}

type ExpandableEvent interface {
	Expand() []Event
}

func (m *MultiObserverEvent) MetricName() string            { return m.OMetricName }
func (m *MultiObserverEvent) Value() float64                { return m.OValues[0] }
func (m *MultiObserverEvent) Labels() map[string]string     { return m.OLabels }
func (m *MultiObserverEvent) MetricType() mapper.MetricType { return mapper.MetricTypeObserver }
func (m *MultiObserverEvent) Values() []float64             { return m.OValues }

// Expand returns a list of events that are the result of expanding the multi-value event.
// This will be used as a middle-step in the pipeline to convert multi-value events to single-value events.
// And keep the exporter code compatible with previous versions.
func (m *MultiObserverEvent) Expand() []Event {
	if len(m.OValues) == 1 && m.SampleRate == 0 {
		return []Event{m}
	}

	events := make([]Event, 0, len(m.OValues))
	for _, value := range m.OValues {
		labels := make(map[string]string, len(m.OLabels))
		for k, v := range m.OLabels {
			labels[k] = v
		}

		events = append(events, &ObserverEvent{
			OMetricName: m.OMetricName,
			OValue:      value,
			OLabels:     labels,
		})
	}

	if m.SampleRate > 0 && m.SampleRate < 1 {
		multiplier := int(1 / m.SampleRate)
		multipliedEvents := make([]Event, 0, len(events)*multiplier)
		for i := 0; i < multiplier; i++ {
			multipliedEvents = append(multipliedEvents, events...)
		}
		return multipliedEvents
	}

	return events
}

var (
	_ ExpandableEvent = &MultiObserverEvent{}
	_ MultiValueEvent = &MultiObserverEvent{}
	_ MultiValueEvent = &CounterEvent{}
	_ MultiValueEvent = &GaugeEvent{}
	_ MultiValueEvent = &ObserverEvent{}
)
