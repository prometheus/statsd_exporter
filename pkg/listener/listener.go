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

package listener

import (
	"bufio"
	"io"
	"net"
	"os"
	"strings"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/statsd_exporter/pkg/event"
	"github.com/prometheus/statsd_exporter/pkg/level"
)

type Parser interface {
	LineToEvents(line string, sampleErrors prometheus.CounterVec, samplesReceived prometheus.Counter, tagErrors prometheus.Counter, tagsReceived prometheus.Counter, logger log.Logger) event.Events
}

type StatsDUDPListener struct {
	Conn         *net.UDPConn
	Logger       log.Logger
	UDPPackets   prometheus.Counter
	PacketBuffer chan string
}

func (l *StatsDUDPListener) SetPacketBuffer(pb chan string) {
	l.PacketBuffer = pb
}

func (l *StatsDUDPListener) Listen() {
	buf := make([]byte, 65535)
	for {
		n, _, err := l.Conn.ReadFromUDP(buf)
		if err != nil {
			// https://github.com/golang/go/issues/4373
			// ignore net: errClosing error as it will occur during shutdown
			if strings.HasSuffix(err.Error(), "use of closed network connection") {
				return
			}
			level.Error(l.Logger).Log("error", err)
			return
		}
		l.HandlePacket(buf[0:n])
	}
}

func (l *StatsDUDPListener) HandlePacket(packet []byte) {
	l.UDPPackets.Inc()
	l.PacketBuffer <- string(packet)
}

type StatsDTCPListener struct {
	Conn         *net.TCPListener
	PacketBuffer chan string
	Logger       log.Logger

	TCPConnections prometheus.Counter
	TCPErrors      prometheus.Counter
	TCPLineTooLong prometheus.Counter
}

func (l *StatsDTCPListener) SetPacketBuffer(pb chan string) {
	l.PacketBuffer = pb
}

func (l *StatsDTCPListener) Listen() {
	for {
		c, err := l.Conn.AcceptTCP()
		if err != nil {
			// https://github.com/golang/go/issues/4373
			// ignore net: errClosing error as it will occur during shutdown
			if strings.HasSuffix(err.Error(), "use of closed network connection") {
				return
			}
			level.Error(l.Logger).Log("msg", "AcceptTCP failed", "error", err)
			os.Exit(1)
		}
		go l.HandleConn(c)
	}
}

func (l *StatsDTCPListener) HandleConn(c *net.TCPConn) {
	defer c.Close()

	l.TCPConnections.Inc()

	r := bufio.NewReader(c)
	for {
		line, isPrefix, err := r.ReadLine()
		if err != nil {
			if err != io.EOF {
				l.TCPErrors.Inc()
				level.Debug(l.Logger).Log("msg", "Read failed", "addr", c.RemoteAddr(), "error", err)
			}
			break
		}
		level.Debug(l.Logger).Log("msg", "Incoming line", "proto", "tcp", "line", line)
		if isPrefix {
			l.TCPLineTooLong.Inc()
			level.Debug(l.Logger).Log("msg", "Read failed: line too long", "addr", c.RemoteAddr())
			break
		}

		l.PacketBuffer <- string(line)
	}
}

type StatsDUnixgramListener struct {
	Conn         *net.UnixConn
	PacketBuffer chan string
	Logger       log.Logger

	UnixgramPackets prometheus.Counter
}

func (l *StatsDUnixgramListener) SetPacketBuffer(pb chan string) {
	l.PacketBuffer = pb
}

func (l *StatsDUnixgramListener) Listen() {
	buf := make([]byte, 65535)
	for {
		n, _, err := l.Conn.ReadFromUnix(buf)
		if err != nil {
			// https://github.com/golang/go/issues/4373
			// ignore net: errClosing error as it will occur during shutdown
			if strings.HasSuffix(err.Error(), "use of closed network connection") {
				return
			}
			level.Error(l.Logger).Log(err)
			os.Exit(1)
		}
		l.HandlePacket(buf[:n])
	}
}

func (l *StatsDUnixgramListener) HandlePacket(packet []byte) {
	l.UnixgramPackets.Inc()
	l.PacketBuffer <- string(packet)
}
