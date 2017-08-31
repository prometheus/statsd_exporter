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
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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
	ex := NewExporter(&metricMapper{}, true, false)

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()

	ex.Listen(events)
}

// TestDropUnmapped validates that the dropUnmapped global will prevent unmapped
// metrics from being recorded when set true and that they are recorded when set
// false.
func TestDropUnmapped(t *testing.T) {
	events := make(chan Events, 1)
	c := Events{
		&CounterEvent{
			metricName: "foo",
			value:      1,
		},
	}
	events <- c
	ex := NewExporter(&metricMapper{}, true, true)

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()

	ex.Listen(events)
	if len(ex.Counters.Elements) != 0 {
		t.Fatalf("Unmapped metric recorded inspite of dropUnmapped being set true")
	}
	ex = NewExporter(&metricMapper{}, true, false)
	events = make(chan Events, 1)
	events <- c
	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()
	ex.Listen(events)
	if len(ex.Counters.Elements) != 1 {
		t.Fatalf("Unmapped metric not recorded inspite of dropUnmapped being set false")
	}
}

// TestInvalidUtf8InDatadogTagValue validates robustness of exporter listener
// against datadog tags with invalid tag values.
// It sends the same tags first with a valid value, then with an invalid one.
// The exporter should not panic, but drop the invalid event
func TestInvalidUtf8InDatadogTagValue(t *testing.T) {
	ex := NewExporter(&metricMapper{}, true, false)
	for _, l := range []statsDPacketHandler{&StatsDUDPListener{}, &mockStatsDTCPListener{}} {
		events := make(chan Events, 2)

		l.handlePacket([]byte("bar:200|c|#tag:value\nbar:200|c|#tag:\xc3\x28invalid"), events)

		// Close channel to signify we are done with the listener after a short period.
		go func() {
			time.Sleep(time.Millisecond * 100)
			close(events)
		}()

		ex.Listen(events)
	}
}

type MockHistogram struct {
	prometheus.Metric
	prometheus.Collector
	value float64
}

func (h *MockHistogram) Observe(n float64) {
	h.value = n
}

func TestHistogramUnits(t *testing.T) {
	events := make(chan Events, 1)
	name := "foo"
	c := Events{
		&TimerEvent{
			metricName: name,
			value:      300,
		},
	}
	events <- c
	ex := NewExporter(&metricMapper{}, true)
	ex.mapper.Defaults.TimerType = timerTypeHistogram

	// Close channel to signify we are done with the listener after a short period.
	go func() {
		time.Sleep(time.Millisecond * 100)
		close(events)
	}()
	mock := &MockHistogram{}
	key := hashNameAndLabels(name+"_timer", nil)
	ex.Histograms.Elements[key] = mock
	ex.Listen(events)
	if mock.value == 300 {
		t.Fatalf("Histogram observations not scaled into Seconds")
	} else if mock.value != .300 {
		t.Fatalf("Received unexpected value for histogram observation %f != .300", mock.value)
	}
}

type statsDPacketHandler interface {
	handlePacket(packet []byte, e chan<- Events)
}

type mockStatsDTCPListener struct {
	StatsDTCPListener
}

func (ml *mockStatsDTCPListener) handlePacket(packet []byte, e chan<- Events) {
	lc, err := net.ListenTCP("tcp", nil)
	if err != nil {
		panic(fmt.Sprintf("mockStatsDTCPListener: listen failed: %v", err))
	}

	defer lc.Close()

	go func() {
		cc, err := net.DialTCP("tcp", nil, lc.Addr().(*net.TCPAddr))
		if err != nil {
			panic(fmt.Sprintf("mockStatsDTCPListener: dial failed: %v", err))
		}

		defer cc.Close()

		n, err := cc.Write(packet)
		if err != nil || n != len(packet) {
			panic(fmt.Sprintf("mockStatsDTCPListener: write failed: %v,%d", err, n))
		}
	}()

	sc, err := lc.AcceptTCP()
	if err != nil {
		panic(fmt.Sprintf("mockStatsDTCPListener: accept failed: %v", err))
	}
	ml.handleConn(sc, e)
}
