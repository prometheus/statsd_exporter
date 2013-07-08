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
	eventStats    = prometheus.NewCounter()
	networkStats  = prometheus.NewCounter()
	configLoads   = prometheus.NewCounter()
	mappingsCount = prometheus.NewGauge()
)

func init() {
	prometheus.Register("statsd_bridge_events_count", "The total number of StatsD events seen.", prometheus.NilLabels, eventStats)
	prometheus.Register("statsd_bridge_packets_count", "The total number of StatsD packets seen.", prometheus.NilLabels, networkStats)
	prometheus.Register("statsd_bridge_config_reloads_count", "The number of configuration reloads.", prometheus.NilLabels, configLoads)
	prometheus.Register("statsd_bridge_loaded_mappings_count", "The number of configured metric mappings.", prometheus.NilLabels, mappingsCount)

}
