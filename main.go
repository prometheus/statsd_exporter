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
	"fmt"
	"github.com/prometheus/statsd_exporter/pkg/telemetry"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/statsd_exporter/pkg/mapper"
)

func init() {
	prometheus.MustRegister(version.NewCollector("statsd_exporter"))
}

func serveHTTP(listenAddress, metricsEndpoint string, logger log.Logger) {
	http.Handle(metricsEndpoint, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>StatsD Exporter</title></head>
			<body>
			<h1>StatsD Exporter</h1>
			<p><a href="` + metricsEndpoint + `">Metrics</a></p>
			</body>
			</html>`))
	})
	level.Error(logger).Log("msg", http.ListenAndServe(listenAddress, nil))
	os.Exit(1)
}

func ipPortFromString(addr string) (*net.IPAddr, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, 0, fmt.Errorf("bad StatsD listening address: %s", addr)
	}

	if host == "" {
		host = "0.0.0.0"
	}
	ip, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return nil, 0, fmt.Errorf("Unable to resolve %s: %s", host, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return nil, 0, fmt.Errorf("Bad port %s: %s", portStr, err)
	}

	return ip, port, nil
}

func udpAddrFromString(addr string) (*net.UDPAddr, error) {
	ip, port, err := ipPortFromString(addr)
	if err != nil {
		return nil, err
	}
	return &net.UDPAddr{
		IP:   ip.IP,
		Port: port,
		Zone: ip.Zone,
	}, nil
}

func tcpAddrFromString(addr string) (*net.TCPAddr, error) {
	ip, port, err := ipPortFromString(addr)
	if err != nil {
		return nil, err
	}
	return &net.TCPAddr{
		IP:   ip.IP,
		Port: port,
		Zone: ip.Zone,
	}, nil
}

func configReloader(fileName string, mapper *mapper.MetricMapper, cacheSize int, logger log.Logger) {

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP)

	for s := range signals {
		if fileName == "" {
			level.Warn(logger).Log("msg", "Received signal but no mapping config to reload", "signal", s)
			continue
		}
		level.Info(logger).Log("msg", "Received signal, attempting reload", "signal", s)
		err := mapper.InitFromFile(fileName, cacheSize)
		if err != nil {
			level.Info(logger).Log("msg", "Error reloading config", "error", err)
			telemetry.ConfigLoads.WithLabelValues("failure").Inc()
		} else {
			level.Info(logger).Log("msg", "Config reloaded successfully")
			telemetry.ConfigLoads.WithLabelValues("success").Inc()
		}
	}
}

func dumpFSM(mapper *mapper.MetricMapper, dumpFilename string, logger log.Logger) error {
	f, err := os.Create(dumpFilename)
	if err != nil {
		return err
	}
	level.Info(logger).Log("msg", "Start dumping FSM", "file_name", dumpFilename)
	w := bufio.NewWriter(f)
	mapper.FSM.DumpFSM(w)
	w.Flush()
	f.Close()
	level.Info(logger).Log("msg", "Finish dumping FSM")
	return nil
}

func main() {
	var (
		listenAddress        = kingpin.Flag("web.listen-address", "The address on which to expose the web interface and generated Prometheus metrics.").Default(":9102").String()
		metricsEndpoint      = kingpin.Flag("web.telemetry-path", "Path under which to expose metrics.").Default("/metrics").String()
		statsdListenUDP      = kingpin.Flag("statsd.listen-udp", "The UDP address on which to receive statsd metric lines. \"\" disables it.").Default(":9125").String()
		statsdListenTCP      = kingpin.Flag("statsd.listen-tcp", "The TCP address on which to receive statsd metric lines. \"\" disables it.").Default(":9125").String()
		statsdListenUnixgram = kingpin.Flag("statsd.listen-unixgram", "The Unixgram socket path to receive statsd metric lines in datagram. \"\" disables it.").Default("").String()
		// not using Int here because flag diplays default in decimal, 0755 will show as 493
		statsdUnixSocketMode = kingpin.Flag("statsd.unixsocket-mode", "The permission mode of the unix socket.").Default("755").String()
		mappingConfig        = kingpin.Flag("statsd.mapping-config", "Metric mapping configuration file name.").String()
		readBuffer           = kingpin.Flag("statsd.read-buffer", "Size (in bytes) of the operating system's transmit read buffer associated with the UDP or Unixgram connection. Please make sure the kernel parameters net.core.rmem_max is set to a value greater than the value specified.").Int()
		cacheSize            = kingpin.Flag("statsd.cache-size", "Maximum size of your metric mapping cache. Relies on least recently used replacement policy if max size is reached.").Default("1000").Int()
		eventQueueSize       = kingpin.Flag("statsd.event-queue-size", "Size of internal queue for processing events").Default("10000").Int()
		eventFlushThreshold  = kingpin.Flag("statsd.event-flush-threshold", "Number of events to hold in queue before flushing").Default("1000").Int()
		eventFlushInterval   = kingpin.Flag("statsd.event-flush-interval", "Number of events to hold in queue before flushing").Default("200ms").Duration()
		dumpFSMPath          = kingpin.Flag("debug.dump-fsm", "The path to dump internal FSM generated for glob matching as Dot file.").Default("").String()
	)

	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("statsd_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	if *statsdListenUDP == "" && *statsdListenTCP == "" && *statsdListenUnixgram == "" {
		level.Error(logger).Log("At least one of UDP/TCP/Unixgram listeners must be specified.")
		os.Exit(1)
	}

	level.Info(logger).Log("msg", "Starting StatsD -> Prometheus Exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "context", version.BuildContext())
	level.Info(logger).Log("msg", "Accepting StatsD Traffic", "udp", *statsdListenUDP, "tcp", *statsdListenTCP, "unixgram", *statsdListenUnixgram)
	level.Info(logger).Log("msg", "Accepting Prometheus Requests", "addr", *listenAddress)

	go serveHTTP(*listenAddress, *metricsEndpoint, logger)

	events := make(chan Events, *eventQueueSize)
	defer close(events)
	eventQueue := newEventQueue(events, *eventFlushThreshold, *eventFlushInterval)

	if *statsdListenUDP != "" {
		udpListenAddr, err := udpAddrFromString(*statsdListenUDP)
		if err != nil {
			level.Error(logger).Log("msg", "invalid UDP listen address", "address", *statsdListenUDP, "error", err)
			os.Exit(1)
		}
		uconn, err := net.ListenUDP("udp", udpListenAddr)
		if err != nil {
			level.Error(logger).Log("msg", "failed to start UDP listener", "error", err)
			os.Exit(1)
		}

		if *readBuffer != 0 {
			err = uconn.SetReadBuffer(*readBuffer)
			if err != nil {
				level.Error(logger).Log("msg", "error setting UDP read buffer", "error", err)
				os.Exit(1)
			}
		}

		ul := &StatsDUDPListener{conn: uconn, eventHandler: eventQueue, logger: logger}
		go ul.Listen()
	}

	if *statsdListenTCP != "" {
		tcpListenAddr, err := tcpAddrFromString(*statsdListenTCP)
		if err != nil {
			level.Error(logger).Log("msg", "invalid TCP listen address", "address", *statsdListenUDP, "error", err)
			os.Exit(1)
		}
		tconn, err := net.ListenTCP("tcp", tcpListenAddr)
		if err != nil {
			level.Error(logger).Log("msg", err)
			os.Exit(1)
		}
		defer tconn.Close()

		tl := &StatsDTCPListener{conn: tconn, eventHandler: eventQueue, logger: logger}
		go tl.Listen()
	}

	if *statsdListenUnixgram != "" {
		var err error
		if _, err = os.Stat(*statsdListenUnixgram); !os.IsNotExist(err) {
			level.Error(logger).Log("msg", "Unixgram socket already exists", "socket_name", *statsdListenUnixgram)
			os.Exit(1)
		}
		uxgconn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{
			Net:  "unixgram",
			Name: *statsdListenUnixgram,
		})
		if err != nil {
			level.Error(logger).Log("msg", "failed to listen on Unixgram socket", "error", err)
			os.Exit(1)
		}

		defer uxgconn.Close()

		if *readBuffer != 0 {
			err = uxgconn.SetReadBuffer(*readBuffer)
			if err != nil {
				level.Error(logger).Log("msg", "error setting Unixgram read buffer", "error", err)
				os.Exit(1)
			}
		}

		ul := &StatsDUnixgramListener{conn: uxgconn, eventHandler: eventQueue, logger: logger}
		go ul.Listen()

		// if it's an abstract unix domain socket, it won't exist on fs
		// so we can't chmod it either
		if _, err := os.Stat(*statsdListenUnixgram); !os.IsNotExist(err) {
			defer os.Remove(*statsdListenUnixgram)

			// convert the string to octet
			perm, err := strconv.ParseInt("0"+string(*statsdUnixSocketMode), 8, 32)
			if err != nil {
				level.Warn(logger).Log("Bad permission %s: %v, ignoring\n", *statsdUnixSocketMode, err)
			} else {
				err = os.Chmod(*statsdListenUnixgram, os.FileMode(perm))
				if err != nil {
					level.Warn(logger).Log("Failed to change unixgram socket permission: %v", err)
				}
			}
		}

	}

	mapper := &mapper.MetricMapper{MappingsCount: telemetry.MappingsCount}
	if *mappingConfig != "" {
		err := mapper.InitFromFile(*mappingConfig, *cacheSize)
		if err != nil {
			level.Error(logger).Log("msg", "error loading config", "error", err)
			os.Exit(1)
		}
		if *dumpFSMPath != "" {
			err := dumpFSM(mapper, *dumpFSMPath, logger)
			if err != nil {
				level.Error(logger).Log("msg", "error dumping FSM", "error", err)
				// Failure to dump the FSM is an error (the user asked for it and it
				// didn't happen) but not fatal (the exporter is fully functional
				// afterwards).
			}
		}
	} else {
		mapper.InitCache(*cacheSize)
	}

	go configReloader(*mappingConfig, mapper, *cacheSize, logger)

	exporter := NewExporter(mapper, logger)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)

	go exporter.Listen(events)

	<-signals
}
