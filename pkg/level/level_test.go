// Copyright 2021 The Prometheus Authors
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

package level

import (
	"bytes"
	"strings"
	"testing"

	"github.com/go-kit/log"
)

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		logLevel Level
		wantErr  bool
	}{
		{"wrong level", "foo", LevelInfo, true},
		{"level debug", "debug", LevelDebug, false},
		{"level info", "info", LevelInfo, false},
		{"level warn", "warn", LevelWarn, false},
		{"level error", "error", LevelError, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SetLogLevel(tt.level); (err != nil) != tt.wantErr {
				t.Fatalf("Expected log level to be set successfully, but got %v", err)
			}
			if tt.logLevel != logLevel {
				t.Fatalf("Expected log level %v, but got %v", tt.logLevel, logLevel)
			}
		})
	}
}

func TestVariousLevels(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  string
	}{
		{
			"level debug",
			"debug",
			strings.Join([]string{
				"level=debug log=debug",
				"level=info log=info",
				"level=warn log=warn",
				"level=error log=error",
			}, "\n"),
		},
		{
			"level info",
			"info",
			strings.Join([]string{
				"level=info log=info",
				"level=warn log=warn",
				"level=error log=error",
			}, "\n"),
		},
		{
			"level warn",
			"warn",
			strings.Join([]string{
				"level=warn log=warn",
				"level=error log=error",
			}, "\n"),
		},
		{
			"level error",
			"error",
			strings.Join([]string{
				"level=error log=error",
			}, "\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := log.NewLogfmtLogger(&buf)

			if err := SetLogLevel(tt.level); err != nil {
				t.Fatalf("Expected log level to be set successfully, but got %v", err)
			}

			Debug(logger).Log("log", "debug")
			Info(logger).Log("log", "info")
			Warn(logger).Log("log", "warn")
			Error(logger).Log("log", "error")

			got := strings.TrimSpace(buf.String())
			if tt.want != got {
				t.Fatalf("Expected log output %v, but got %v", tt.want, got)
			}
		})
	}
}
