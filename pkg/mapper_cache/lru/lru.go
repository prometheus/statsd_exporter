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

package lru

import (
	"github.com/prometheus/client_golang/prometheus"

	lru2 "github.com/hashicorp/golang-lru"

	"github.com/prometheus/statsd_exporter/pkg/mapper_cache"
)

type metricMapperLRUCache struct {
	cache   *lru2.Cache
	metrics *mapper_cache.CacheMetrics
}

func NewMetricMapperLRUCache(reg prometheus.Registerer, size int) (*metricMapperLRUCache, error) {
	if size <= 0 {
		return nil, nil
	}

	metrics := mapper_cache.NewCacheMetrics(reg)
	cache, err := lru2.New(size)
	if err != nil {
		return &metricMapperLRUCache{}, err
	}

	return &metricMapperLRUCache{metrics: metrics, cache: cache}, nil
}

func (m *metricMapperLRUCache) Get(metricKey string) (interface{}, bool) {
	m.metrics.CacheGetsTotal.Inc()
	if result, ok := m.cache.Get(metricKey); ok {
		m.metrics.CacheHitsTotal.Inc()
		return result, true
	} else {
		return nil, false
	}
}

func (m *metricMapperLRUCache) Add(metricKey string, result interface{}) {
	go m.trackCacheLength()
	m.cache.Add(metricKey, result)
}

func (m *metricMapperLRUCache) trackCacheLength() {
	m.metrics.CacheLength.Set(float64(m.cache.Len()))
}

func (m *metricMapperLRUCache) Reset() {
	m.cache.Purge()
	m.metrics.CacheLength.Set(0)
}
