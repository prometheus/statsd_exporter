// +build linux

package telemetry

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

const SOCK_DIAG_BY_FAMILY = 20

type inetDiagSockId struct {
	SourcePort    uint16
	DestPort      uint16
	SourceAddress [4]uint32
	DestAddress   [4]uint32
	If            uint32
	Cookie        [2]uint32
}

type inetDiagReqV2 struct {
	Family   uint8
	Protocol uint8
	Ext      uint8
	_        uint8
	States   uint32
	Id       inetDiagSockId
}

type InetDiagMsgData struct {
	Family  uint8
	State   uint8
	Timer   uint8
	Retrans uint8
	Id      inetDiagSockId
	Expires uint32
	Rqueue  uint32
	Wqueue  uint32
	Uid     uint32
	Inode   uint32
}

// MarshalBinary marshals a Message into a byte slice.
func (m inetDiagReqV2) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, &m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func convert_addr_to_int32(ip []byte) (ret [4]uint32) {
	buf := bytes.NewBuffer(ip)
	binary.Read(buf, binary.BigEndian, &ret)
	return
}

func convert_port_to_u16(port int) uint16 {
	uport := uint16(port)
	portdat := make([]byte, 6)
	binary.LittleEndian.PutUint16(portdat, uport)
	uport = binary.BigEndian.Uint16(portdat)
	return uport
}

func (b *BufferWatcher) GetSocketQueue() (int, error) {
	c, err := netlink.Dial(unix.NETLINK_SOCK_DIAG, nil)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	r := inetDiagReqV2{
		Family:   unix.AF_INET6,
		Protocol: unix.IPPROTO_UDP,
		Ext:      255,
		Id: inetDiagSockId{
			SourcePort:    convert_port_to_u16(b.uAddr.Port),
			SourceAddress: convert_addr_to_int32(b.uAddr.IP),
		},
		States: 0xffffffff,
	}

	data, err := r.MarshalBinary()
	if err != nil {
		return 0, err
	}

	req := netlink.Message{
		Header: netlink.Header{
			Flags: netlink.Root | netlink.Match |
				netlink.Request,
			Type: SOCK_DIAG_BY_FAMILY,
		},
		Data: data,
	}

	// Perform a request, receive replies, and validate the replies
	msgs, err := c.Execute(req)
	if err != nil {
		panic(err)
	}

	msg_count := len(msgs)

	if msg_count == 1 {
		m := msgs[0]
		var data *InetDiagMsgData = *(**InetDiagMsgData)(unsafe.Pointer(&m.Data))

		return int(data.Rqueue), nil
	} else {
		return 0, errors.New("Netlink returned an unexpected number of sockets: " + strconv.Itoa(msg_count))
	}
}

func getReadBuffer(uConn *net.UDPConn) (int, error) {
	file, err := uConn.File()
	if err != nil {
		return 0, err
	}

	readBuffer, err := unix.GetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	if err != nil {
		return 0, err
	}
	return readBuffer, nil
}

func NewBufferWatcher(uConn *net.UDPConn) (*BufferWatcher, error) {
	readBuffer, err := getReadBuffer(uConn)
	if err != nil {
		return &BufferWatcher{}, err
	}

	return &BufferWatcher{
		ReadBuffer: readBuffer,
		uAddr:      uConn.LocalAddr().(*net.UDPAddr),
	}, nil
}
