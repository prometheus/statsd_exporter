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

package main

import (
	"sync"
	"time"

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
	metricName string
	value      float64
	labels     map[string]string
}

func (c *CounterEvent) MetricName() string            { return c.metricName }
func (c *CounterEvent) Value() float64                { return c.value }
func (c *CounterEvent) Labels() map[string]string     { return c.labels }
func (c *CounterEvent) MetricType() mapper.MetricType { return mapper.MetricTypeCounter }

type GaugeEvent struct {
	metricName string
	value      float64
	relative   bool
	labels     map[string]string
}

func (g *GaugeEvent) MetricName() string            { return g.metricName }
func (g *GaugeEvent) Value() float64                { return g.value }
func (c *GaugeEvent) Labels() map[string]string     { return c.labels }
func (c *GaugeEvent) MetricType() mapper.MetricType { return mapper.MetricTypeGauge }

type TimerEvent struct {
	metricName string
	value      float64
	labels     map[string]string
}

func (t *TimerEvent) MetricName() string            { return t.metricName }
func (t *TimerEvent) Value() float64                { return t.value }
func (c *TimerEvent) Labels() map[string]string     { return c.labels }
func (c *TimerEvent) MetricType() mapper.MetricType { return mapper.MetricTypeTimer }

type Events []Event

type eventQueue struct {
	c              chan Events
	q              Events
	m              sync.Mutex
	flushThreshold int
	flushTicker    *time.Ticker
}

type eventHandler interface {
	queue(event Events)
}

func newEventQueue(c chan Events, flushThreshold int, flushInterval time.Duration) *eventQueue {
	ticker := clock.NewTicker(flushInterval)
	eq := &eventQueue{
		c:              c,
		flushThreshold: flushThreshold,
		flushTicker:    ticker,
		q:              make([]Event, 0, flushThreshold),
	}
	go func() {
		for {
			<-ticker.C
			eq.flush()
		}
	}()
	return eq
}

func (eq *eventQueue) queue(events Events) {
	eq.m.Lock()
	defer eq.m.Unlock()

	for _, e := range events {
		eq.q = append(eq.q, e)
		if len(eq.q) >= eq.flushThreshold {
			eq.flushUnlocked()
		}
	}
}

func (eq *eventQueue) flush() {
	eq.m.Lock()
	defer eq.m.Unlock()
	eq.flushUnlocked()
}

func (eq *eventQueue) flushUnlocked() {
	eq.c <- eq.q
	eq.q = make([]Event, 0, cap(eq.q))
	eventsFlushed.Inc()
}

func (eq *eventQueue) len() int {
	eq.m.Lock()
	defer eq.m.Unlock()

	return len(eq.q)
}

type unbufferedEventHandler struct {
	c chan Events
}

func (ueh *unbufferedEventHandler) queue(events Events) {
	ueh.c <- events
}
