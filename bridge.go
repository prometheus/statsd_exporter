// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	illegalCharsRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
)

type CounterContainer struct {
	Elements map[string]prometheus.Counter
}

func NewCounterContainer() *CounterContainer {
	return &CounterContainer{
		Elements: make(map[string]prometheus.Counter),
	}
}

func (c *CounterContainer) Get(metricName string) prometheus.Counter {
	counter, ok := c.Elements[metricName]
	if !ok {
		counter = prometheus.NewCounter()
		c.Elements[metricName] = counter
		prometheus.Register(metricName, "", prometheus.NilLabels, counter)
	}
	return counter
}

type GaugeContainer struct {
	Elements map[string]prometheus.Gauge
}

func NewGaugeContainer() *GaugeContainer {
	return &GaugeContainer{
		Elements: make(map[string]prometheus.Gauge),
	}
}

func (c *GaugeContainer) Get(metricName string) prometheus.Gauge {
	gauge, ok := c.Elements[metricName]
	if !ok {
		gauge = prometheus.NewGauge()
		c.Elements[metricName] = gauge
		prometheus.Register(metricName, "", prometheus.NilLabels, gauge)
	}
	return gauge
}

type SummaryContainer struct {
	Elements map[string]prometheus.Histogram
}

func NewSummaryContainer() *SummaryContainer {
	return &SummaryContainer{
		Elements: make(map[string]prometheus.Histogram),
	}
}

func (c *SummaryContainer) Get(metricName string) prometheus.Histogram {
	summary, ok := c.Elements[metricName]
	if !ok {
		summary = prometheus.NewDefaultHistogram()
		c.Elements[metricName] = summary
		prometheus.Register(metricName, "", prometheus.NilLabels, summary)
	}
	return summary
}

func (c *SummaryContainer) Flush() {
	for _, summary := range c.Elements {
		summary.ResetAll()
	}
}

type Event interface {
	MetricName() string
	Value() float64
}

type CounterEvent struct {
	metricName string
	value      float64
}

func (c *CounterEvent) MetricName() string { return c.metricName }
func (c *CounterEvent) Value() float64     { return c.value }

type GaugeEvent struct {
	metricName string
	value      float64
}

func (g *GaugeEvent) MetricName() string { return g.metricName }
func (g *GaugeEvent) Value() float64     { return g.value }

type TimerEvent struct {
	metricName string
	value      float64
}

func (t *TimerEvent) MetricName() string { return t.metricName }
func (t *TimerEvent) Value() float64     { return t.value }

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
			prometheusLabels := map[string]string{}

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
				counter := b.Counters.Get(metricName + "_counter")
				counter.IncrementBy(prometheusLabels, event.Value())

				eventStats.Increment(map[string]string{"type": "counter"})

			case *GaugeEvent:
				gauge := b.Gauges.Get(metricName + "_gauge")
				gauge.Set(prometheusLabels, event.Value())

				eventStats.Increment(map[string]string{"type": "gauge"})

			case *TimerEvent:
				summary := b.Summaries.Get(metricName + "_timer")
				summary.Add(prometheusLabels, event.Value())

				sum := b.Counters.Get(metricName + "_timer_total")
				sum.IncrementBy(prometheusLabels, event.Value())

				count := b.Counters.Get(metricName + "_timer_count")
				count.Increment(prometheusLabels)

				eventStats.Increment(map[string]string{"type": "timer"})

			default:
				log.Println("Unsupported event type")
				eventStats.Increment(map[string]string{"type": "illegal"})
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

func buildEvent(statType, metric string, value float64) (Event, error) {
	switch statType {
	case "c":
		return &CounterEvent{
			metricName: metric,
			value:      float64(value),
		}, nil
	case "g":
		return &GaugeEvent{
			metricName: metric,
			value:      float64(value),
		}, nil
	case "ms":
		return &TimerEvent{
			metricName: metric,
			value:      float64(value),
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

		elements := strings.Split(line, ":")
		if len(elements) < 2 {
			networkStats.Increment(map[string]string{"type": "malformed_line"})
			log.Println("Bad line from StatsD:", line)
			continue
		}
		metric := elements[0]
		samples := elements[1:]
		for _, sample := range samples {
			components := strings.Split(sample, "|")
			samplingFactor := 1.0
			if len(components) < 2 || len(components) > 3 {
				networkStats.Increment(map[string]string{"type": "malformed_component"})
				log.Println("Bad component on line:", line)
				continue
			}
			valueStr, statType := components[0], components[1]
			value, err := strconv.ParseFloat(valueStr, 64)
			if err != nil {
				log.Printf("Bad value %s on line: %s", valueStr, line)
				networkStats.Increment(map[string]string{"type": "malformed_value"})
				continue
			}

			if len(components) == 3 {
				if statType != "c" {
					log.Println("Illegal sampling factor for non-counter metric on line", line)
					networkStats.Increment(map[string]string{"type": "illegal_sample_factor"})
				}
				samplingStr := components[2]
				if samplingStr[0] != '@' {
					log.Printf("Invalid sampling factor %s on line %s", samplingStr, line)
					networkStats.Increment(map[string]string{"type": "invalid_sample_factor"})
					continue
				}
				samplingFactor, err = strconv.ParseFloat(samplingStr[1:], 64)
				if err != nil {
					log.Printf("Invalid sampling factor %s on line %s", samplingStr, line)
					networkStats.Increment(map[string]string{"type": "invalid_sample_factor"})
					continue
				}
				if samplingFactor == 0 {
					// This should never happen, but avoid division by zero if it does.
					log.Printf("Invalid zero sampling factor %s on line %s, setting to 1", samplingStr, line)
					samplingFactor = 1
				}
				value /= samplingFactor
			}

			event, err := buildEvent(statType, metric, value)
			if err != nil {
				log.Printf("Error building event on line %s: %s", line, err)
				networkStats.Increment(map[string]string{"type": "illegal_event"})
				continue
			}
			events = append(events, event)
			networkStats.Increment(map[string]string{"type": "legal"})
		}
	}
	e <- events
}
