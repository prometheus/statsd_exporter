// Copyright 2013 The Prometheus Authors
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
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	eventStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_total",
			Help: "The total number of StatsD events seen.",
		},
		[]string{"type"},
	)
	eventsUnmapped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "statsd_exporter_events_unmapped_total",
		Help: "The total number of StatsD events no mapping was found for.",
	})
	udpPackets = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_udp_packets_total",
			Help: "The total number of StatsD packets received over UDP.",
		},
	)
	tcpConnections = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tcp_connections_total",
			Help: "The total number of TCP connections handled.",
		},
	)
	tcpErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tcp_connection_errors_total",
			Help: "The number of errors encountered reading from TCP.",
		},
	)
	tcpLineTooLong = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tcp_too_long_lines_total",
			Help: "The number of lines discarded due to being too long.",
		},
	)
	linesReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_lines_total",
			Help: "The total number of StatsD lines received.",
		},
	)
	samplesReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_samples_total",
			Help: "The total number of StatsD samples received.",
		},
	)
	sampleErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_sample_errors_total",
			Help: "The total number of errors parsing StatsD samples.",
		},
		[]string{"reason"},
	)
	tagsReceived = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tags_total",
			Help: "The total number of DogStatsD tags processed.",
		},
	)
	tagErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_exporter_tag_errors_total",
			Help: "The number of errors parsign DogStatsD tags.",
		},
	)
	configLoads = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_config_reloads_total",
			Help: "The number of configuration reloads.",
		},
		[]string{"outcome"},
	)
	mappingsCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "statsd_exporter_loaded_mappings",
		Help: "The current number of configured metric mappings.",
	})
	conflictingEventStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_events_conflict_total",
			Help: "The total number of StatsD events with conflicting names.",
		},
		[]string{"type"},
	)
	udpBufferQueued = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "statsd_exporter_udp_buffer_queued",
			Help: "The number of queued UDP messages in the linux buffer.",
		},
		[]string{"protocol"},
	)
	udpBufferDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "statsd_exporter_udp_buffer_dropped",
			Help: "The number of dropped UDP messages in the linux buffer",
		},
		[]string{"protocol"},
	)
)

func init() {
	prometheus.MustRegister(eventStats)
	prometheus.MustRegister(eventsUnmapped)
	prometheus.MustRegister(udpPackets)
	prometheus.MustRegister(tcpConnections)
	prometheus.MustRegister(tcpErrors)
	prometheus.MustRegister(tcpLineTooLong)
	prometheus.MustRegister(linesReceived)
	prometheus.MustRegister(samplesReceived)
	prometheus.MustRegister(sampleErrors)
	prometheus.MustRegister(tagsReceived)
	prometheus.MustRegister(tagErrors)
	prometheus.MustRegister(configLoads)
	prometheus.MustRegister(mappingsCount)
	prometheus.MustRegister(conflictingEventStats)
	if runtime.GOOS == "linux" {
		prometheus.MustRegister(udpBufferQueued)
		prometheus.MustRegister(udpBufferDropped)
	}
}

func watchUDPBuffers(lastDropped int, lastDropped6 int) {
	myPid := strconv.Itoa(os.Getpid())

	queuedUDP, droppedUDP, err := parseProcfsNetFile("/proc/" + myPid + "/net/udp")
	if err != nil {
		log.Info("Encountered error while scraping UDP stats. Will not continue scraping stats.", err)
		return
	}

	label := "udp"

	udpBufferQueued.WithLabelValues(label).Set(float64(queuedUDP))

	diff := droppedUDP - lastDropped
	if diff < 0 {
		log.Info("Dropped count went negative! Abandoning UDP buffer parsing")
		diff = 0
		droppedUDP = lastDropped
	}
	udpBufferDropped.WithLabelValues(label).Add(float64(diff))

	queuedUDP6, droppedUDP6, err := parseProcfsNetFile("/proc/" + myPid + "/net/udp6")
	if err != nil {
		log.Info("Encountered error while scraping UDP stats. Will not continue scraping stats.", err)
		return
	}
	label = "udp6"

	udpBufferQueued.WithLabelValues(label).Set(float64(queuedUDP6))

	diff = droppedUDP6 - lastDropped6
	if diff < 0 {
		log.Info("Dropped count went negative! Abandoning UDP buffer parsing")
		diff = 0
		droppedUDP6 = lastDropped6
	}
	udpBufferDropped.WithLabelValues(label).Add(float64(diff))

	time.Sleep(10 * time.Second)
	watchUDPBuffers(droppedUDP, droppedUDP6)
}

func parseProcfsNetFile(filename string) (int, int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	queued := 0
	dropped := 0
	s := bufio.NewScanner(f)
	for n := 0; s.Scan(); n++ {
		// Skip the header lines.
		if n < 1 {
			continue
		}

		fields := strings.Fields(s.Text())

		queuedLine, err := strconv.ParseInt(strings.Split(fields[4], ":")[1], 16, 32)
		queued = queued + int(queuedLine)
		if err != nil {
			log.Info("Unable to parse queued UDP buffers:", err)
			return 0, 0, err
		}

		droppedLine, err := strconv.Atoi(fields[12])
		dropped = dropped + droppedLine
		if err != nil {
			log.Info("Unable to parse dropped UDP buffers:", err)
			return 0, 0, err
		}
	}

	return queued, dropped, nil
}
