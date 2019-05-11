package pftp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	DATA_TRANSFER_BUFFER_SIZE = 4096
	LISTENER_TIMEOUT          = 30
)

type dataListener struct {
	responseCode  string
	listener      *net.TCPListener
	txConn        net.Conn
	rxConn        net.Conn
	remoteAddr    string
	remoteIP      string
	remotePort    int
	keepaliveTime int
	isConnected   bool
	receivedIP    string
	listenerPort  int
	log           *logger
}

// Make listener for data connection
func newDataListener(line string, receivedIP string, keepaliveTime int, log *logger, resCode string) (string, int, error) {
	var err error

	d := &dataListener{
		responseCode:  resCode,
		receivedIP:    receivedIP,
		listener:      nil,
		txConn:        nil,
		rxConn:        nil,
		keepaliveTime: keepaliveTime,
		isConnected:   false,
		log:           log,
	}
	lAddr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:")
	if err != nil {
		d.log.err("cannot resolve TCPAddr")
		return "", -1, err
	}

	if d.listener, err = net.ListenTCP("tcp4", lAddr); err != nil {
		d.log.err("cannot open data channel socket: %v", err)
		return "", -1, err
	}
	// set listener timeout
	d.listener.SetDeadline(time.Now().Add(time.Duration(LISTENER_TIMEOUT) * time.Second))

	// get listen port
	d.listenerPort, _ = strconv.Atoi(strings.SplitN(d.listener.Addr().String(), ":", 2)[1])

	startIndex := strings.Index(line, "(")
	endIndex := strings.LastIndex(line, ")")

	switch d.responseCode {
	case "PASV":
		// PASV response format : "227 Entering Passive Mode (h1,h2,h3,h4,p1,p2)."
		if startIndex == -1 || endIndex == -1 {
			err = errors.New("Invalid PASV response format")
			return "", -1, err
		}
		d.remoteIP, d.remotePort = parseToAddr(line[startIndex+1 : endIndex])
	case "EPSV":
		// EPSV response format : "229 Entering Extended Passive Mode (|||port|)"
		if startIndex == -1 || endIndex == -1 {
			err = errors.New("Invalid PASV response format")
			return "", -1, err
		}
		originPort := strings.Trim(line[startIndex+1:endIndex], "|")
		d.remoteIP = d.receivedIP
		d.remotePort, _ = strconv.Atoi(originPort)
	case "PORT":
		// PORT command format : "PORT h1,h2,h3,h4,p1,p2\r\n"
		line = strings.Split(strings.Trim(line, "\r\n"), " ")[1]
		d.remoteIP, d.remotePort = parseToAddr(line)
	}

	// start listener & listener timer
	go d.startDataListener()

	return d.receivedIP, d.listenerPort, nil
}

// Make listener for data connection
func (d *dataListener) startDataListener() error {
	var err error

	eg := errgroup.Group{}

	defer func() {
		// close each net.Conn absolutely(for end goroutine)
		d.rxConn.Close()
		d.txConn.Close()

		// close listener
		d.listener.Close()
	}()

	d.rxConn, err = d.listener.Accept()
	if err != nil {
		d.log.err("listen error : %s", err.Error())
		return err
	}
	// close listener immediatly
	d.listener.Close()
	d.listener = nil

	d.log.info("Data channel connected from %s", d.rxConn.RemoteAddr().String())

	// set conn to TCPConn
	tcpConn := d.rxConn.(*net.TCPConn)

	if d.keepaliveTime > 0 {
		// set linger 0 and tcp keepalive setting between client connection
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Duration(d.keepaliveTime) * time.Second)
		tcpConn.SetLinger(0)

		// set deadline
		tcpConn.SetDeadline(time.Now().Add(time.Duration(d.keepaliveTime) * time.Second))
	}

	err = d.connectToRemoteDataChan()
	if err != nil {
		return err
	}
	// txConn to rxConn
	eg.Go(func() error { return d.dataTransfer(d.rxConn, d.txConn) })

	// rxConn to txConn
	eg.Go(func() error { return d.dataTransfer(d.txConn, d.rxConn) })

	// wait until data transfer goroutine end
	if err = eg.Wait(); err != nil {
		d.log.err(err.Error())
	}

	d.log.debug("proxy data channel disconnected")

	return nil
}

// Connect to origin server data channel
func (d *dataListener) connectToRemoteDataChan() error {
	var err error

	d.txConn, err = net.Dial("tcp", net.JoinHostPort(d.remoteIP, strconv.Itoa(d.remotePort)))
	if err != nil {
		return err
	}

	// set conn to TCPConn
	tcpConn := d.txConn.(*net.TCPConn)

	// set linger 0 and tcp keepalive setting between client connection
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(d.keepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	// set deadline
	tcpConn.SetDeadline(time.Now().Add(time.Duration(d.keepaliveTime) * time.Second))

	return nil
}

// send data until got EOF
func (d *dataListener) dataTransfer(reader net.Conn, writer net.Conn) error {
	var err error

	buffer := make([]byte, DATA_TRANSFER_BUFFER_SIZE)
	if bytes, err := io.CopyBuffer(writer, reader, buffer); err == nil {
		d.log.debug("data transfer in porxy successs with %d byte", bytes)
		err = nil
	} else {
		err = fmt.Errorf("got error on data transfer: %s", err.Error())
	}

	// send EOF to writer
	shutdownWrite(writer)

	return err
}

// parse PASV IP and Port from server response
func parseToAddr(line string) (string, int) {
	var ip string = ""

	addr := strings.Split(line, ",")
	for i := 0; i < 4; i++ {
		ip += addr[i]
		if i < 3 {
			ip += "."
		}
	}

	// Let's compute the port number
	port1, _ := strconv.Atoi(addr[4])
	port2, _ := strconv.Atoi(addr[5])

	return ip, (port1 * 256) + port2
}

// send EOF to write
func shutdownWrite(conn net.Conn) {
	// anonymous interface. Could explicitly use TCP instead.
	if v, ok := conn.(interface{ CloseWrite() error }); ok {
		v.CloseWrite()
	}
}
