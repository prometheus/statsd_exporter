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
	"testing"
	"time"

	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/prometheus/statsd_exporter/pkg/event"
)

func TestEventThresholdFlush(t *testing.T) {
	c := make(chan event.Events, 100)
	// We're not going to flush during this test, so the duration doesn't matter.
	eq := event.NewEventQueue(c, 5, time.Second, eventsFlushed)
	e := make(event.Events, 13)
	go func() {
		eq.Queue(e, &eventsFlushed)
	}()

	batch := <-c
	if len(batch) != 5 {
		t.Fatalf("Expected event batch to be 5 elements, but got %v", len(batch))
	}
	batch = <-c
	if len(batch) != 5 {
		t.Fatalf("Expected event batch to be 5 elements, but got %v", len(batch))
	}
	batch = <-c
	if len(batch) != 3 {
		t.Fatalf("Expected event batch to be 3 elements, but got %v", len(batch))
	}
}

func TestEventIntervalFlush(t *testing.T) {
	// Mock a time.NewTicker
	tickerCh := make(chan time.Time)
	clock.ClockInstance = &clock.Clock{
		TickerCh: tickerCh,
	}
	clock.ClockInstance.Instant = time.Unix(0, 0)

	c := make(chan event.Events, 100)
	eq := event.NewEventQueue(c, 1000, time.Second*1000, eventsFlushed)
	e := make(event.Events, 10)
	eq.Queue(e, &eventsFlushed)

	if eq.Len() != 10 {
		t.Fatal("Expected 10 events to be queued, but got", eq.Len())
	}

	if len(eq.C) != 0 {
		t.Fatal("Expected 0 events in the event channel, but got", len(eq.C))
	}

	// Tick time forward to trigger a flush
	clock.ClockInstance.Instant = time.Unix(10000, 0)
	clock.ClockInstance.TickerCh <- time.Unix(10000, 0)

	events := <-eq.C
	if eq.Len() != 0 {
		t.Fatal("Expected 0 events to be queued, but got", eq.Len())
	}

	if len(events) != 10 {
		t.Fatal("Expected 10 events in the event channel, but got", len(events))
	}

}
