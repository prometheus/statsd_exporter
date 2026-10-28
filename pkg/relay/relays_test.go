// Copyright 2026 The Prometheus Authors
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
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/statsd_exporter/pkg/clock"
)

func TestRelays_RelayLine(t *testing.T) {
	restoreClock(t)

	conn1, addr1 := startUDPReceiver(t)
	conn2, addr2 := startUDPReceiver(t)
	defer conn1.Close()
	defer conn2.Close()

	logger := promslog.NewNopLogger()
	r1, err := NewRelay(logger, addr1, 200)
	if err != nil {
		t.Fatalf("creating relay 1: %v", err)
	}
	r2, err := NewRelay(logger, addr2, 200)
	if err != nil {
		t.Fatalf("creating relay 2: %v", err)
	}

	relays := NewRelays([]*Relay{r1, r2})
	line := "foo:1|c"
	relays.RelayLine(line)
	// A second line that does not fit in the remaining packet budget flushes
	// the first line without waiting for the 1s relay ticker.
	relays.RelayLine(strings.Repeat("x", 192))
	waitBufferDrained(r1)
	waitBufferDrained(r2)

	want := line + "\n"
	if got := readUDP(t, conn1); got != want {
		t.Errorf("relay 1 received %q, want %q", got, want)
	}
	if got := readUDP(t, conn2); got != want {
		t.Errorf("relay 2 received %q, want %q", got, want)
	}
}

func TestRelays_RelayLineRecoversPanic(t *testing.T) {
	restoreClock(t)

	conn, addr := startUDPReceiver(t)
	defer conn.Close()

	logger := promslog.NewNopLogger()
	r, err := NewRelay(logger, addr, 200)
	if err != nil {
		t.Fatalf("creating relay: %v", err)
	}

	relays := NewRelays([]*Relay{nil, r})
	line := "foo:1|c"
	relays.RelayLine(line)
	relays.RelayLine(strings.Repeat("x", 192))
	waitBufferDrained(r)

	want := line + "\n"
	if got := readUDP(t, conn); got != want {
		t.Errorf("healthy relay received %q, want %q", got, want)
	}
}

func restoreClock(t *testing.T) {
	t.Helper()
	oldClock := clock.ClockInstance
	clock.ClockInstance = nil
	t.Cleanup(func() {
		clock.ClockInstance = oldClock
	})
}

func startUDPReceiver(t *testing.T) (*net.UDPConn, string) {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve udp: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	return conn, conn.LocalAddr().String()
}

func waitBufferDrained(r *Relay) {
	for goSchedTimes := 0; goSchedTimes < 1000; goSchedTimes++ {
		if len(r.bufferChannel) == 0 {
			return
		}
		runtime.Gosched()
	}
}

func readUDP(t *testing.T, conn *net.UDPConn) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read udp: %v", err)
	}
	return string(buf[:n])
}
