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

package line

import (
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"
)

// maxExampleLen caps how much of an offending line ends up in a warning, so a
// single huge datagram cannot flood the log on its own.
const maxExampleLen = 256

// warnLimiter lets one warning through per key and interval and counts the
// ones it held back. The keys are the fixed error reasons, so the map stays
// small.
type warnLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	now      func() time.Time
	last     map[string]time.Time
	skipped  map[string]int
}

func newWarnLimiter(interval time.Duration, now func() time.Time) *warnLimiter {
	return &warnLimiter{
		interval: interval,
		now:      now,
		last:     map[string]time.Time{},
		skipped:  map[string]int{},
	}
}

// allow reports whether a warning for key may be logged now. When it may, it
// also returns how many warnings for the key were held back since the last one.
func (l *warnLimiter) allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if last, ok := l.last[key]; ok && now.Sub(last) < l.interval {
		l.skipped[key]++
		return false, 0
	}
	l.last[key] = now
	skipped := l.skipped[key]
	delete(l.skipped, key)
	return true, skipped
}

// warnMalformed logs a rate limited warning about input that was dropped. The
// first problem of each reason is logged right away, later ones within the
// interval are only counted.
func (p *Parser) warnMalformed(logger *slog.Logger, reason, example string) {
	if p.limiter == nil {
		return
	}
	ok, skipped := p.limiter.allow(reason)
	if !ok {
		return
	}
	logger.Warn("Dropped malformed StatsD input",
		"reason", reason,
		"example", truncate(example, maxExampleLen),
		"suppressed_since_last_warning", skipped,
		"interval", p.limiter.interval.String(),
	)
}

// truncate shortens s to at most n bytes without cutting a rune in half.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}
