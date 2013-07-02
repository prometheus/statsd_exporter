// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/exp"
)

var listeningAddress = flag.String("listeningAddress", ":8080", "The address on which to expose generated Prometheus metrics.")
var statsdListeningAddress = flag.String("statsdListeningAddress", ":8126", "The UDP address on which to receive statsd metric lines.")

type CounterContainer struct {
	sync.RWMutex

	Elements map[string]prometheus.Counter
}

func NewCounterContainer() *CounterContainer {
	return &CounterContainer{
		Elements: make(map[string]prometheus.Counter),
	}
}

func (c *CounterContainer) Get(metricName string) prometheus.Counter {
	c.Lock()
	defer c.Unlock()

	counter, ok := c.Elements[metricName]
	if !ok {
		counter = prometheus.NewCounter()
		c.Elements[metricName] = counter
		prometheus.Register(metricName, "", prometheus.NilLabels, counter)
	}
	return counter
}

type GaugeContainer struct {
	sync.RWMutex

	Elements map[string]prometheus.Gauge
}

func NewGaugeContainer() *GaugeContainer {
	return &GaugeContainer{
		Elements: make(map[string]prometheus.Gauge),
	}
}

func (c *GaugeContainer) Get(metricName string) prometheus.Gauge {
	c.Lock()
	defer c.Unlock()

	gauge, ok := c.Elements[metricName]
	if !ok {
		gauge = prometheus.NewGauge()
		c.Elements[metricName] = gauge
		prometheus.Register(metricName, "", prometheus.NilLabels, gauge)
	}
	return gauge
}

type SummaryContainer struct {
	sync.RWMutex

	Elements map[string]prometheus.Histogram

	ResetInterval time.Duration
}

func NewSummaryContainer() *SummaryContainer {
	return &SummaryContainer{
		Elements: make(map[string]prometheus.Histogram),
	}
}

func (c *SummaryContainer) Get(metricName string) prometheus.Histogram {
	c.Lock()
	defer c.Unlock()

	summary, ok := c.Elements[metricName]
	if !ok {
		summary = prometheus.NewDefaultHistogram()
		c.Elements[metricName] = summary
		prometheus.Register(metricName, "", prometheus.NilLabels, summary)
	}
	return summary
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
}

func escapeMetricName(metricName string) string {
	// TODO: evaluate what kind of escaping we really want.
	metricName = strings.Replace(metricName, "_", "__", -1)
	metricName = strings.Replace(metricName, "-", "__", -1)
	metricName = strings.Replace(metricName, ".", "_", -1)
	return metricName
}

func (b *Bridge) Listen(e <-chan Events) {
	for {
		events := <-e
		for _, event := range events {
			metricName := escapeMetricName(event.MetricName())
			switch event.(type) {
			case *CounterEvent:
				counter := b.Counters.Get(metricName + "_counter")
				counter.IncrementBy(prometheus.NilLabels, event.Value())
			case *GaugeEvent:
				gauge := b.Gauges.Get(metricName + "_gauge")
				gauge.Set(prometheus.NilLabels, event.Value())
			case *TimerEvent:
				summary := b.Summaries.Get(metricName + "_timer")
				summary.Add(prometheus.NilLabels, event.Value())
			default:
				log.Println("Unsupported event type")
			}
		}
	}
}

func NewBridge() *Bridge {
	return &Bridge{
		Counters:  NewCounterContainer(),
		Gauges:    NewGaugeContainer(),
		Summaries: NewSummaryContainer(),
	}
}

type StatsDListener struct {
	conn *net.UDPConn
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
			log.Println("Bad line from StatsD:", line)
			continue
		}
		metric := elements[0]
		samples := elements[1:]
		for _, sample := range samples {
			components := strings.Split(sample, "|")
			samplingFactor := 1.0
			if len(components) < 2 || len(components) > 3 {
				log.Println("Bad component on line:", line)
				continue
			}
			valueStr, statType := components[0], components[1]
			value, err := strconv.Atoi(valueStr)
			if err != nil {
				log.Printf("Bad value %s on line: %s", valueStr, line)
				continue
			}

			if len(components) == 3 {
				if statType != "c" {
					log.Println("Illegal sampling factor for non-counter metric on line", line)
				}
				samplingStr := components[2]
				if samplingStr[0] != '@' {
					log.Printf("Invalid sampling factor %s on line %s", samplingStr, line)
					continue
				}
				samplingFactor, err = strconv.ParseFloat(samplingStr[1:], 64)
				if err != nil {
					log.Printf("Invalid sampling factor %s on line %s", samplingStr, line)
					continue
				}
				if samplingFactor == 0 {
					// This should never happen, but avoid division by zero if it does.
					log.Printf("Invalid zero sampling factor %s on line %s, setting to 1", samplingStr, line)
					samplingFactor = 1
				}
			}

			var event Event
			switch statType {
			case "c":
				event = &CounterEvent{
					metricName: metric,
					value:      float64(value) / samplingFactor,
				}
			case "g":
				event = &GaugeEvent{
					metricName: metric,
					value:      float64(value),
				}
			case "ms":
				event = &TimerEvent{
					metricName: metric,
					value:      float64(value),
				}
			case "s":
				log.Println("No support for StatsD sets in line", line)
			default:
				log.Printf("Bad stat type %s on line: %s", statType, line)
			}
			if event != nil {
				events = append(events, event)
			}
		}
	}
	e <- events
}

func serveHTTP() {
	exp.Handle(prometheus.ExpositionResource, prometheus.DefaultHandler)
	http.ListenAndServe(*listeningAddress, exp.DefaultCoarseMux)
}

func udpAddrFromString(addr string) *net.UDPAddr {
	host, portStr, err := net.SplitHostPort(*statsdListeningAddress)
	if err != nil {
		log.Fatal("Bad StatsD listening address", *statsdListeningAddress)
	}

	if host == "" {
		host = "0.0.0.0"
	}
	ip, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		log.Fatalf("Unable to resolve %s: %s", host, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		log.Fatal("Bad port %s: %s", portStr, err)
	}

	return &net.UDPAddr{
		IP:   ip.IP,
		Port: port,
		Zone: ip.Zone,
	}
}

func main() {
	flag.Parse()

	go serveHTTP()

	events := make(chan Events, 1024)
	defer close(events)

	listenAddr := udpAddrFromString(*statsdListeningAddress)
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	l := &StatsDListener{conn: conn}
	go l.Listen(events)

	bridge := NewBridge()
	bridge.Listen(events)
}
