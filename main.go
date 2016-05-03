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
	"flag"
	"net"
	"net/http"
	"strconv"

	"github.com/howeyc/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
)

var (
	// Version of statsd_exporter. Set at build time.
	Version = "0.0.0.dev"

	listenAddress       = flag.String("web.listen-address", ":9102", "The address on which to expose the web interface and generated Prometheus metrics.")
	metricsEndpoint     = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	statsdListenAddress = flag.String("statsd.listen-address", ":9125", "The UDP address on which to receive statsd metric lines.")
	mappingConfig       = flag.String("statsd.mapping-config", "", "Metric mapping configuration file name.")
	readBuffer          = flag.Int("statsd.read-buffer", 0, "Size (in bytes) of the operating system's transmit read buffer associated with the UDP connection. Please make sure the kernel parameters net.core.rmem_max is set to a value greater than the value specified.")
	addSuffix           = flag.Bool("statsd.add-suffix", true, "Add the metric type (counter/gauge/timer) as suffix to the generated Prometheus metric (NOT recommended, but set by default for backward compatibility).")
)

func serveHTTP() {
	http.Handle(*metricsEndpoint, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>StatsD Bridge</title></head>
			<body>
			<h1>StatsD Bridge</h1>
			<p><a href="` + *metricsEndpoint + `">Metrics</a></p>
			</body>
			</html>`))
	})
	http.ListenAndServe(*listenAddress, nil)
}

func udpAddrFromString(addr string) *net.UDPAddr {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		log.Fatal("Bad StatsD listening address", addr)
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
		log.Fatalf("Bad port %s: %s", portStr, err)
	}

	return &net.UDPAddr{
		IP:   ip.IP,
		Port: port,
		Zone: ip.Zone,
	}
}

func watchConfig(fileName string, mapper *metricMapper) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	err = watcher.WatchFlags(fileName, fsnotify.FSN_MODIFY)
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case ev := <-watcher.Event:
			log.Printf("Config file changed (%s), attempting reload", ev)
			err = mapper.initFromFile(fileName)
			if err != nil {
				log.Println("Error reloading config:", err)
				configLoads.WithLabelValues("failure").Inc()
			} else {
				log.Println("Config reloaded successfully")
				configLoads.WithLabelValues("success").Inc()
			}
			// Re-add the file watcher since it can get lost on some changes. E.g.
			// saving a file with vim results in a RENAME-MODIFY-DELETE event
			// sequence, after which the newly written file is no longer watched.
			err = watcher.WatchFlags(fileName, fsnotify.FSN_MODIFY)
		case err := <-watcher.Error:
			log.Println("Error watching config:", err)
		}
	}
}

func main() {
	flag.Parse()

	if *addSuffix {
		log.Println("Warning: Using -statsd.add-suffix is discouraged. We recommend explicitly naming metrics appropriately in the mapping configuration.")
	}
	log.Printf("Starting StatsD -> Prometheus Exporter v%s...", Version)
	log.Println("Accepting StatsD Traffic on", *statsdListenAddress)
	log.Println("Accepting Prometheus Requests on", *listenAddress)

	go serveHTTP()

	events := make(chan Events, 1024)
	defer close(events)

	listenAddr := udpAddrFromString(*statsdListenAddress)
	conn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}

	if *readBuffer != 0 {
		err = conn.SetReadBuffer(*readBuffer)
		if err != nil {
			log.Fatal("Error setting UDP read buffer:", err)
		}
	}

	l := &StatsDListener{conn: conn}
	go l.Listen(events)

	mapper := &metricMapper{}
	if *mappingConfig != "" {
		err := mapper.initFromFile(*mappingConfig)
		if err != nil {
			log.Fatal("Error loading config:", err)
		}
		go watchConfig(*mappingConfig, mapper)
	}
	exporter := NewExporter(mapper, *addSuffix)
	exporter.Listen(events)
}
