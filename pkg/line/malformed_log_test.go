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
	"testing"
	"testing/synctest"
	"time"
)

func countWarnings(buf *bytes.Buffer) int {
	return strings.Count(buf.String(), "level=WARN")
}

func parseWith(p *Parser, logger *slog.Logger, line string) {
	p.LineToEvents(line, *nopSampleErrors, nopSamplesReceived, nopTagErrors, nopTagsReceived, logger)
}

func TestMalformedWarningsPerMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		p := NewParser()
		p.EnableDogstatsdParsing()
		p.SetMalformedLogPerMinute(3)

		for range 10 {
			parseWith(p, logger, "no colon here")
		}
		if got := countWarnings(&buf); got != 3 {
			t.Fatalf("got %d warnings in the first burst, want 3", got)
		}
		if !strings.Contains(buf.String(), `example="no colon here"`) {
			t.Errorf("warning has no example:\n%s", buf.String())
		}

		// One token comes back every 20 seconds.
		time.Sleep(20 * time.Second)
		for range 10 {
			parseWith(p, logger, "m:abc|c")
		}
		if got := countWarnings(&buf); got != 4 {
			t.Fatalf("got %d warnings after 20s, want 4", got)
		}

		time.Sleep(time.Minute)
		for range 10 {
			parseWith(p, logger, "m:1|c|#novalue")
		}
		if got := countWarnings(&buf); got != 7 {
			t.Fatalf("got %d warnings after a minute, want 7", got)
		}
	})
}

func TestMalformedWarningsOffByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p := NewParser()
	parseWith(p, logger, "no colon here")
	p.SetMalformedLogPerMinute(0)
	parseWith(p, logger, "no colon here")
	if got := countWarnings(&buf); got != 0 {
		t.Fatalf("got %d warnings, want 0", got)
	}
}

func TestMalformedWarningExampleIsCapped(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	p := NewParser()
	p.SetMalformedLogPerMinute(1)
	parseWith(p, logger, strings.Repeat("x", 1000))
	if got := buf.Len(); got > 400 {
		t.Fatalf("warning is %d bytes, example was not capped", got)
	}
}
