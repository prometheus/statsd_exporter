// Copyright 2020 The Prometheus Authors
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

package mapper

import "testing"

func TestEscapeMetricName(t *testing.T) {
	scenarios := map[string]string{
		"clean":                   "clean",
		"0starts_with_digit":      "_0starts_with_digit",
		"with_underscore":         "with_underscore",
		"with.dot":                "with_dot",
		"with😱emoji":              "with_emoji",
		"with.*.multiple":         "with___multiple",
		"test.web-server.foo.bar": "test_web_server_foo_bar",
		"":                        "",
	}

	for in, want := range scenarios {
		if got := EscapeMetricName(in); want != got {
			t.Errorf("expected `%s` to be escaped to `%s`, got `%s`", in, want, got)
		}
	}
}

func BenchmarkEscapeMetricName(b *testing.B) {
	scenarios := []string{
		"clean",
		"0starts_with_digit",
		"with_underscore",
		"with.dot",
		"with😱emoji",
		"with.*.multiple",
		"test.web-server.foo.bar",
		"",
	}

	for _, s := range scenarios {
		b.Run(s, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				EscapeMetricName(s)
			}
		})
	}
}
