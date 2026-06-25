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

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/common/promslog"
)

type fakeMetricsClearer struct {
	cleared int
	called  int
}

func (f *fakeMetricsClearer) ClearMetrics() int {
	f.called++
	return f.cleared
}

func TestClearMetricsHandler(t *testing.T) {
	clearer := &fakeMetricsClearer{cleared: 3}
	handler := clearMetricsHandler(clearer, promslog.NewNopLogger())

	request := httptest.NewRequest(http.MethodPost, "/-/clear", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if clearer.called != 1 {
		t.Fatalf("expected clearer to be called once, got %d", clearer.called)
	}
	if body := response.Body.String(); !strings.Contains(body, "Cleared 3 metric series") {
		t.Fatalf("unexpected response body: %q", body)
	}
}
