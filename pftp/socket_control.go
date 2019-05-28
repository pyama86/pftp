package pftp

import (
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type closer interface {
	Close() error
}

// send EOF to write
func sendEOF(conn net.Conn) error {
	// anonymous interface. Could explicitly use TCP instead.
	if v, ok := conn.(interface{ CloseWrite() error }); ok {
		if err := v.CloseWrite(); err != nil {
			if !strings.Contains(err.Error(), AlreadyClosedMsg) {
				return err
			}
		}
	}

	return nil
}

// close connection
func connectionCloser(c closer, log *logger) {
	if err := c.Close(); err != nil {
		if !strings.Contains(err.Error(), AlreadyClosedMsg) {
			// log is nil when unit test
			if log != nil {
				log.err(err.Error())
			}
		}
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
