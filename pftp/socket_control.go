package pftp

import (
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// send EOF to write
func sendEOF(conn net.Conn) {
	// anonymous interface. Could explicitly use TCP instead.
	if v, ok := conn.(interface{ CloseWrite() error }); ok {
		v.CloseWrite()
	}
}

// set reuse IP & Port for sharing port 20 (just set active mode)
func setReuseIPPort(network, address string, c syscall.RawConn) error {
	var err error
	c.Control(func(fd uintptr) {
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if err != nil {
			return
		}

		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if err != nil {
			return
		}
	})
	return err
}
