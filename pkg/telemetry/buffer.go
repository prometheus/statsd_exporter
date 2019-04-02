package telemetry

import (
	"net"
)

type BufferWatcher struct {
	uAddr      *net.UDPAddr
	ReadBuffer int
}
