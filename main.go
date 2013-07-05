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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/exp"
)

var (
	listeningAddress       = flag.String("listeningAddress", ":8080", "The address on which to expose generated Prometheus metrics.")
	statsdListeningAddress = flag.String("statsdListeningAddress", ":9125", "The UDP address on which to receive statsd metric lines.")
	mappingConfig          = flag.String("mappingConfig", "mapping.conf", "Metric mapping configuration file name.")
	summaryFlushInterval   = flag.Duration("summaryFlushInterval", 15*time.Minute, "How frequently to reset all summary metrics.")
)

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

	log.Println("Starting StatsD -> Prometheus Bridge...")
	log.Println("Accepting StatsD Traffic on", *statsdListeningAddress)
	log.Println("Accepting Prometheus Requests on", *listeningAddress)

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

	mapper := metricMapper{}
	if mappingConfig != nil {
		err := mapper.initFromFile(*mappingConfig)
		if err != nil {
			log.Fatal("Error loading config:", err)
		}
	}
	bridge := NewBridge(&mapper)
	go func() {
		for _ = range time.Tick(*summaryFlushInterval) {
			bridge.Summaries.Flush()
		}
	}()
	bridge.Listen(events)
}
