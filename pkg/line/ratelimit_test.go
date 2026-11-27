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

package line

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestWarnLimiter(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1000, 0)}
	l := newWarnLimiter(time.Minute, clock.now)

	if ok, skipped := l.allow("a"); !ok || skipped != 0 {
		t.Fatalf("first call: got (%v, %d), want (true, 0)", ok, skipped)
	}
	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("a"); ok {
			t.Fatalf("call %d inside the interval was allowed", i)
		}
	}
	if ok, skipped := l.allow("b"); !ok || skipped != 0 {
		t.Fatalf("other key: got (%v, %d), want (true, 0)", ok, skipped)
	}

	clock.advance(59 * time.Second)
	if ok, _ := l.allow("a"); ok {
		t.Fatal("allowed before the interval passed")
	}
	clock.advance(time.Second)
	if ok, skipped := l.allow("a"); !ok || skipped != 4 {
		t.Fatalf("after interval: got (%v, %d), want (true, 4)", ok, skipped)
	}
	if ok, skipped := l.allow("a"); ok || skipped != 0 {
		t.Fatalf("right after a warning: got (%v, %d), want (false, 0)", ok, skipped)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("got %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc..." {
		t.Errorf("got %q", got)
	}
	// Each rune is three bytes, a cut at byte 4 would split the second one.
	if got := truncate("世界世界", 4); got != "世..." {
		t.Errorf("got %q", got)
	}
}

func warnLines(buf *bytes.Buffer) []string {
	var out []string
	for _, l := range strings.Split(buf.String(), "\n") {
		if strings.Contains(l, "level=WARN") {
			out = append(out, l)
		}
	}
	return out
}

func newBufferLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestMalformedWarningsAreRateLimited(t *testing.T) {
	var buf bytes.Buffer
	logger := newBufferLogger(&buf)

	clock := &fakeClock{t: time.Unix(1000, 0)}
	p := NewParser()
	p.EnableDogstatsdParsing()
	p.limiter = newWarnLimiter(time.Minute, clock.now)

	parse := func(line string) {
		p.LineToEvents(line, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, logger)
	}

	for i := 0; i < 5; i++ {
		parse("no colon here")
	}
	parse("m:1|c|#novalue")
	parse("m:abc|c")

	warns := warnLines(&buf)
	if len(warns) != 3 {
		t.Fatalf("got %d warnings, want 3:\n%s", len(warns), strings.Join(warns, "\n"))
	}
	for i, want := range []string{"reason=malformed_line", "reason=malformed_tag", "reason=malformed_value"} {
		if !strings.Contains(warns[i], want) {
			t.Errorf("warning %d: want %q in %q", i, want, warns[i])
		}
	}
	if !strings.Contains(warns[0], `example="no colon here"`) {
		t.Errorf("missing example in %q", warns[0])
	}

	buf.Reset()
	clock.advance(time.Minute)
	parse("still no colon")
	warns = warnLines(&buf)
	if len(warns) != 1 {
		t.Fatalf("got %d warnings after the interval, want 1", len(warns))
	}
	if !strings.Contains(warns[0], "suppressed_since_last_warning=4") {
		t.Errorf("want 4 suppressed in %q", warns[0])
	}
}

func TestMalformedWarningsOffByDefault(t *testing.T) {
	var buf bytes.Buffer
	p := NewParser()
	p.LineToEvents("no colon here", *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, newBufferLogger(&buf))
	if warns := warnLines(&buf); len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if !strings.Contains(buf.String(), "level=DEBUG") {
		t.Error("debug log for the bad line is missing")
	}

	p.SetMalformedLogInterval(time.Minute)
	p.LineToEvents("no colon here", *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, newBufferLogger(&buf))
	if warns := warnLines(&buf); len(warns) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warns))
	}
	p.SetMalformedLogInterval(0)
	if p.limiter != nil {
		t.Error("a zero interval should disable the warnings")
	}
}

func TestMalformedWarningsConcurrent(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	logger := slog.New(slog.NewTextHandler(&lockedWriter{mu: &mu, w: &buf}, nil))

	p := NewParser()
	p.SetMalformedLogInterval(time.Hour)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				p.LineToEvents("no colon here", *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, logger)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if warns := warnLines(&buf); len(warns) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warns))
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}
