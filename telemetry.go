// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	eventStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_bridge_events_total",
			Help: "The total number of StatsD events seen.",
		},
		[]string{"type"},
	)
	networkStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_bridge_packets_total",
			Help: "The total number of StatsD packets seen.",
		},
		[]string{"type"},
	)
	configLoads = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_bridge_config_reloads_total",
			Help: "The number of configuration reloads.",
		},
		[]string{"outcome"},
	)
	mappingsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "statsd_bridge_loaded_mappings_count",
		Help: "The number of configured metric mappings.",
	})
)

func init() {
	prometheus.MustRegister(eventStats)
	prometheus.MustRegister(networkStats)
	prometheus.MustRegister(configLoads)
	prometheus.MustRegister(mappingsCount)
}
