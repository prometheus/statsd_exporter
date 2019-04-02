// +build !linux

package telemetry

import (
	"errors"
	"net"
)

func NewBufferWatcher(uConn *net.UDPConn) (*BufferWatcher, error) {
	return &BufferWatcher{}, errors.New("UDP Buffer watching unsupported on this OS")
}

func (b *BufferWatcher) GetSocketQueue() (int, error) {
	return 0, errors.New("UDP Buffer watching unsupported on this OS")
}
