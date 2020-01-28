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

package expiringregistry

import "testing"

func TestHashLabelNames(t *testing.T) {
	r := NewRegistry(nil, nil)
	// Validate value hash changes and name has doesn't when just the value changes.
	hash1, _ := r.hashLabels(map[string]string{
		"label": "value1",
	})
	hash2, _ := r.hashLabels(map[string]string{
		"label": "value2",
	})
	if hash1.names != hash2.names {
		t.Fatal("Hash of label names should match, but doesn't")
	}
	if hash1.values == hash2.values {
		t.Fatal("Hash of label names shouldn't match, but do")
	}

	// Validate value and name hashes change when the name changes.
	hash1, _ = r.hashLabels(map[string]string{
		"label1": "value",
	})
	hash2, _ = r.hashLabels(map[string]string{
		"label2": "value",
	})
	if hash1.names == hash2.names {
		t.Fatal("Hash of label names shouldn't match, but do")
	}
	if hash1.values == hash2.values {
		t.Fatal("Hash of label names shouldn't match, but do")
	}
}

func BenchmarkHashNameAndLabels(b *testing.B) {
	scenarios := []struct {
		name   string
		metric string
		labels map[string]string
	}{
		{
			name:   "no labels",
			labels: map[string]string{},
		}, {
			name: "one label",
			labels: map[string]string{
				"label": "value",
			},
		}, {
			name: "many labels",
			labels: map[string]string{
				"label0": "value",
				"label1": "value",
				"label2": "value",
				"label3": "value",
				"label4": "value",
				"label5": "value",
				"label6": "value",
				"label7": "value",
				"label8": "value",
				"label9": "value",
			},
		},
	}

	r := NewRegistry(nil, nil)
	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				r.hashLabels(s.labels)
			}
		})
	}
}
