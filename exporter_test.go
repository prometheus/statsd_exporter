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
)

// TestNegativeCounter validates when we send a negative
// number to a counter that we no longer panic the Exporter Listener.
func TestNegativeCounter(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			err := e.(error)
			if err.Error() == "counter cannot decrease in value" {
				t.Fatalf("Counter was negative and causes a panic.")
			} else {
				t.Fatalf("Unknown panic and error: %q", err.Error())
			}
		}
	}()

	events := make(chan Events, 1)
	c := Events{
		&CounterEvent{
			metricName: "foo",
			value:      -1,
		},
	}
	events <- c
	ex := NewExporter(&metricMapper{}, true)

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()

	ex.Listen(events)
}

// TestInvalidUtf8InDatadogTagValue validates robustness of exporter listener
// against datadog tags with invalid tag values.
// It sends the same tags first with a valid value, then with an invalid one.
// The exporter should not panic, but drop the invalid event
func TestInvalidUtf8InDatadogTagValue(t *testing.T) {
	l := StatsDListener{}
	events := make(chan Events, 2)

	l.handlePacket([]byte("bar:200|c|#tag:value"), events)
	l.handlePacket([]byte("bar:200|c|#tag:\xc3\x28invalid"), events)

	ex := NewExporter(&metricMapper{}, true)

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()

	ex.Listen(events)
}
