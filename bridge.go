// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/model"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	defaultHelp = "Metric autogenerated by statsd_bridge."
	regErrF     = "A change of configuration created inconsistent metrics for " +
		"%q. You have to restart the statsd_bridge, and you should " +
		"consider the effects on your monitoring setup. Error: %s"
)

var (
	illegalCharsRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)

	hash   = fnv.New64a()
	strBuf bytes.Buffer // Used for hashing.
	intBuf = make([]byte, 8)
)

// hashNameAndLabels returns a hash value of the provided name string and all
// the label names and values in the provided labels map.
//
// Not safe for concurrent use! (Uses a shared buffer and hasher to save on
// allocations.)
func hashNameAndLabels(name string, labels prometheus.Labels) uint64 {
	hash.Reset()
	strBuf.Reset()
	strBuf.WriteString(name)
	hash.Write(strBuf.Bytes())
	binary.BigEndian.PutUint64(intBuf, model.LabelsToSignature(labels))
	hash.Write(intBuf)
	return hash.Sum64()
}

type CounterContainer struct {
	Elements map[uint64]prometheus.Counter
}

func NewCounterContainer() *CounterContainer {
	return &CounterContainer{
		Elements: make(map[uint64]prometheus.Counter),
	}
}

func (c *CounterContainer) Get(metricName string, labels prometheus.Labels) prometheus.Counter {
	hash := hashNameAndLabels(metricName, labels)
	counter, ok := c.Elements[hash]
	if !ok {
		counter = prometheus.NewCounter(prometheus.CounterOpts{
			Name:        metricName,
			Help:        defaultHelp,
			ConstLabels: labels,
		})
		c.Elements[hash] = counter
		if _, err := prometheus.Register(counter); err != nil {
			log.Fatalf(regErrF, metricName, err)
		}
	}
	return counter
}

type GaugeContainer struct {
	Elements map[uint64]prometheus.Gauge
}

func NewGaugeContainer() *GaugeContainer {
	return &GaugeContainer{
		Elements: make(map[uint64]prometheus.Gauge),
	}
}

func (c *GaugeContainer) Get(metricName string, labels prometheus.Labels) prometheus.Gauge {
	hash := hashNameAndLabels(metricName, labels)
	gauge, ok := c.Elements[hash]
	if !ok {
		gauge = prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        metricName,
			Help:        defaultHelp,
			ConstLabels: labels,
		})
		c.Elements[hash] = gauge
		if _, err := prometheus.Register(gauge); err != nil {
			log.Fatalf(regErrF, metricName, err)
		}
	}
	return gauge
}

type SummaryContainer struct {
	Elements map[uint64]prometheus.Summary
}

func NewSummaryContainer() *SummaryContainer {
	return &SummaryContainer{
		Elements: make(map[uint64]prometheus.Summary),
	}
}

func (c *SummaryContainer) Get(metricName string, labels prometheus.Labels) prometheus.Summary {
	hash := hashNameAndLabels(metricName, labels)
	summary, ok := c.Elements[hash]
	if !ok {
		summary = prometheus.NewSummary(
			prometheus.SummaryOpts{
				Name:        metricName,
				Help:        defaultHelp,
				ConstLabels: labels,
			})
		c.Elements[hash] = summary
		if _, err := prometheus.Register(summary); err != nil {
			log.Fatalf(regErrF, metricName, err)
		}
	}
	return summary
}

type Event interface {
	MetricName() string
	Value() float64
	Labels() map[string]string
}

type CounterEvent struct {
	metricName string
	value      float64
	labels     map[string]string
}

func (c *CounterEvent) MetricName() string        { return c.metricName }
func (c *CounterEvent) Value() float64            { return c.value }
func (c *CounterEvent) Labels() map[string]string { return c.labels }

type GaugeEvent struct {
	metricName string
	value      float64
	labels     map[string]string
}

func (g *GaugeEvent) MetricName() string        { return g.metricName }
func (g *GaugeEvent) Value() float64            { return g.value }
func (c *GaugeEvent) Labels() map[string]string { return c.labels }

type TimerEvent struct {
	metricName string
	value      float64
	labels     map[string]string
}

func (t *TimerEvent) MetricName() string        { return t.metricName }
func (t *TimerEvent) Value() float64            { return t.value }
func (c *TimerEvent) Labels() map[string]string { return c.labels }

type Events []Event

type Bridge struct {
	Counters  *CounterContainer
	Gauges    *GaugeContainer
	Summaries *SummaryContainer
	mapper    *metricMapper
}

func escapeMetricName(metricName string) string {
	// If a metric starts with a digit, prepend an underscore.
	if metricName[0] >= '0' && metricName[0] <= '9' {
		metricName = "_" + metricName
	}

	// Replace all illegal metric chars with underscores.
	metricName = illegalCharsRE.ReplaceAllString(metricName, "_")
	return metricName
}

func (b *Bridge) Listen(e <-chan Events) {
	for {
		events := <-e
		for _, event := range events {
			metricName := ""
			prometheusLabels := event.Labels()

			labels, present := b.mapper.getMapping(event.MetricName())
			if present {
				metricName = labels["name"]
				for label, value := range labels {
					if label != "name" {
						prometheusLabels[label] = value
					}
				}
			} else {
				metricName = escapeMetricName(event.MetricName())
			}

			switch event.(type) {
			case *CounterEvent:
				counter := b.Counters.Get(
					metricName+"_counter",
					prometheusLabels,
				)
				counter.Add(event.Value())

				eventStats.WithLabelValues("counter").Inc()

			case *GaugeEvent:
				gauge := b.Gauges.Get(
					metricName+"_gauge",
					prometheusLabels,
				)
				gauge.Set(event.Value())

				eventStats.WithLabelValues("gauge").Inc()

			case *TimerEvent:
				summary := b.Summaries.Get(
					metricName+"_timer",
					prometheusLabels,
				)
				summary.Observe(event.Value())

				eventStats.WithLabelValues("timer").Inc()

			default:
				log.Println("Unsupported event type")
				eventStats.WithLabelValues("illegal").Inc()
			}
		}
	}
}

func NewBridge(mapper *metricMapper) *Bridge {
	return &Bridge{
		Counters:  NewCounterContainer(),
		Gauges:    NewGaugeContainer(),
		Summaries: NewSummaryContainer(),
		mapper:    mapper,
	}
}

type StatsDListener struct {
	conn *net.UDPConn
}

func buildEvent(statType, metric string, value float64, labels map[string]string) (Event, error) {
	switch statType {
	case "c":
		return &CounterEvent{
			metricName: metric,
			value:      float64(value),
			labels:     labels,
		}, nil
	case "g":
		return &GaugeEvent{
			metricName: metric,
			value:      float64(value),
			labels:     labels,
		}, nil
	case "ms", "h":
		return &TimerEvent{
			metricName: metric,
			value:      float64(value),
			labels:     labels,
		}, nil
	case "s":
		return nil, fmt.Errorf("No support for StatsD sets")
	default:
		return nil, fmt.Errorf("Bad stat type %s", statType)
	}
}

func (l *StatsDListener) Listen(e chan<- Events) {
	// TODO: evaluate proper size according to MTU
	var buf [512]byte
	for {
		n, _, err := l.conn.ReadFromUDP(buf[0:])
		if err != nil {
			log.Fatal(err)
		}
		l.handlePacket(buf[0:n], e)
	}
}

func (l *StatsDListener) handlePacket(packet []byte, e chan<- Events) {
	lines := strings.Split(string(packet), "\n")
	events := Events{}
	for _, line := range lines {
		if line == "" {
			continue
		}

		elements := strings.SplitN(line, ":", 2)
		if len(elements) < 2 {
			networkStats.WithLabelValues("malformed_line").Inc()
			log.Println("Bad line from StatsD:", line)
			continue
		}
		metric := elements[0]
		var samples []string
		if strings.Contains(elements[1], "|#") {
			// using datadog extensions, disable multi-metrics
			samples = elements[1:]
		} else {
			samples = strings.Split(elements[1], ":")
		}
		for _, sample := range samples {
			components := strings.Split(sample, "|")
			samplingFactor := 1.0
			if len(components) < 2 || len(components) > 4 {
				networkStats.WithLabelValues("malformed_component").Inc()
				log.Println("Bad component on line:", line)
				continue
			}
			valueStr, statType := components[0], components[1]
			labels := map[string]string{}
			value, err := strconv.ParseFloat(valueStr, 64)
			if err != nil {
				log.Printf("Bad value %s on line: %s", valueStr, line)
				networkStats.WithLabelValues("malformed_value").Inc()
				continue
			}

			if len(components) >= 3 {
				for _, component := range components[2:] {
					switch component[0] {
					case '@':
						if statType != "c" {
							log.Println("Illegal sampling factor for non-counter metric on line", line)
							networkStats.WithLabelValues("illegal_sample_factor").Inc()
						}
						samplingFactor, err = strconv.ParseFloat(component[1:], 64)
						if err != nil {
							log.Printf("Invalid sampling factor %s on line %s", component[1:], line)
							networkStats.WithLabelValues("invalid_sample_factor").Inc()
						}
						if samplingFactor == 0 {
							samplingFactor = 1
						}
						value /= samplingFactor
					case '#':
						networkStats.WithLabelValues("dogstasd_tags").Inc()
						tags := strings.Split(component[1:], ",")
						for _, t := range tags {
							kv := strings.Split(t, ":")
							if len(kv) == 2 {
								if len(kv[1]) > 0 {
									labels[kv[0]] = kv[1]
								}
							} else if len(kv) == 1 {
								labels[kv[0]] = "."
							}
						}
					default:
						log.Printf("Invalid sampling factor or tag section %s on line %s", components[2], line)
						networkStats.WithLabelValues("invalid_sample_factor").Inc()
						continue
					}
				}
			}

			event, err := buildEvent(statType, metric, value, labels)
			if err != nil {
				log.Printf("Error building event on line %s: %s", line, err)
				networkStats.WithLabelValues("illegal_event").Inc()
				continue
			}
			events = append(events, event)
			networkStats.WithLabelValues("legal").Inc()
		}
	}
	e <- events
}
