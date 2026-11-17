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

// Relays fans out RelayLine calls to multiple Relay targets.
type Relays struct {
	targets []*Relay
}

// NewRelays creates a fan-out wrapper around the provided relays.
func NewRelays(targets []*Relay) *Relays {
	return &Relays{targets: targets}
}

// RelayLine forwards a statsd line to each target independently.
// RelayLine on *Relay only enqueues, so calling targets sequentially does not
// wait for UDP sends. A panic from one target is recovered so the others still run.
func (rs *Relays) RelayLine(l string) {
	for _, r := range rs.targets {
		relayTo(r, l)
	}
}

func relayTo(r *Relay, l string) {
	defer func() {
		if rec := recover(); rec != nil && r != nil && r.logger != nil {
			r.logger.Error("panic while relaying line", "panic", rec)
		}
	}()
	r.RelayLine(l)
}
