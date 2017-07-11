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
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/howeyc/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
)

func init() {
	prometheus.MustRegister(version.NewCollector("statsd_exporter"))
}

var (
	listenAddress       = flag.String("web.listen-address", ":9102", "The address on which to expose the web interface and generated Prometheus metrics.")
	metricsEndpoint     = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	statsdListenAddress = flag.String("statsd.listen-address", ":9125", "The UDP/TCP listen address on which to receive statsd metric lines.")
	statsdListenTCP     = flag.Bool("statsd.listen-tcp", false, "Whether to use TCP for statsd metric. Default to false, i.e UDP.")
	mappingConfig       = flag.String("statsd.mapping-config", "", "Metric mapping configuration file name.")
	readBuffer          = flag.Int("statsd.read-buffer", 0, "Size (in bytes) of the operating system's transmit read buffer associated with the UDP connection. Please make sure the kernel parameters net.core.rmem_max is set to a value greater than the value specified.")
	addSuffix           = flag.Bool("statsd.add-suffix", true, "Add the metric type (counter/gauge/timer) as suffix to the generated Prometheus metric (NOT recommended, but set by default for backward compatibility).")
	showVersion         = flag.Bool("version", false, "Print version information.")
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
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}

func ipPortFromString(addr string) (*net.IPAddr, int) {
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

	return ip, port
}

func udpAddrFromString(addr string) *net.UDPAddr {
	ip, port := ipPortFromString(addr)
	return &net.UDPAddr{
		IP:   ip.IP,
		Port: port,
		Zone: ip.Zone,
	}
}

func tcpAddrFromString(addr string) *net.TCPAddr {
	ip, port := ipPortFromString(addr)
	return &net.TCPAddr{
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
			log.Infof("Config file changed (%s), attempting reload", ev)
			err = mapper.initFromFile(fileName)
			if err != nil {
				log.Errorln("Error reloading config:", err)
				configLoads.WithLabelValues("failure").Inc()
			} else {
				log.Infoln("Config reloaded successfully")
				configLoads.WithLabelValues("success").Inc()
			}
			// Re-add the file watcher since it can get lost on some changes. E.g.
			// saving a file with vim results in a RENAME-MODIFY-DELETE event
			// sequence, after which the newly written file is no longer watched.
			err = watcher.WatchFlags(fileName, fsnotify.FSN_MODIFY)
		case err := <-watcher.Error:
			log.Errorln("Error watching config:", err)
		}
	}
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Fprintln(os.Stdout, version.Print("statsd_exporter"))
		os.Exit(0)
	}

	if *addSuffix {
		log.Warnln("Warning: Using -statsd.add-suffix is discouraged. We recommend explicitly naming metrics appropriately in the mapping configuration.")
	}
	log.Infoln("Starting StatsD -> Prometheus Exporter", version.Info())
	log.Infoln("Build context", version.BuildContext())
	log.Infof("Accepting StatsD Traffic on %s, TCP %v", *statsdListenAddress, *statsdListenTCP)
	log.Infoln("Accepting Prometheus Requests on", *listenAddress)

	go serveHTTP()

	events := make(chan Events, 1024)
	defer close(events)

	if *statsdListenTCP {
		tcpListenAddr := tcpAddrFromString(*statsdListenAddress)
		tconn, err := net.ListenTCP("tcp", tcpListenAddr)
		if err != nil {
			log.Fatal(err)
		}
		defer tconn.Close()

		tl := &StatsDTCPListener{conn: tconn}
		go tl.Listen(events)
	} else {
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
	}

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
