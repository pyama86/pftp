package pftp

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
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
	responseCode string
	listener     *net.TCPListener
	txConn       net.Conn
	rxConn       net.Conn
	remoteAddr   string
	remoteIP     string
	remotePort   int
	isConnected  bool
	receivedIP   string
	listenerPort int
	config       *config
	log          *logger
}

// Make listener for data connection
func newDataListener(line string, receivedIP string, config *config, log *logger, resCode string) (string, int, error) {
	var err error
	var lAddr *net.TCPAddr

	d := &dataListener{
		responseCode: resCode,
		receivedIP:   receivedIP,
		listener:     nil,
		txConn:       nil,
		rxConn:       nil,
		isConnected:  false,
		config:       config,
		log:          log,
	}

	// reallocate listener port when selected port is busy until LISTEN_TIMEOUT
	counter := 0
	for {
		lPort := d.getListenPort()

		switch lPort {
		case 0:
			lAddr, err = net.ResolveTCPAddr("tcp", "0.0.0.0:")
		default:
			lAddr, err = net.ResolveTCPAddr("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(lPort)))
		}
		if err != nil {
			d.log.err("cannot resolve TCPAddr")
			return "", -1, err
		}

		if d.listener, err = net.ListenTCP("tcp4", lAddr); err != nil {
			if counter > LISTENER_TIMEOUT {
				d.log.err("cannot make data port")

				return "", -1, err
			}

			d.log.debug("cannot use choosen port. try to select another port after 1 second...")

			time.Sleep(time.Duration(1) * time.Second)
			continue

		} else {
			// set listener timeout
			d.listener.SetDeadline(time.Now().Add(time.Duration(LISTENER_TIMEOUT) * time.Second))

			// get listen port
			d.listenerPort, _ = strconv.Atoi(strings.SplitN(d.listener.Addr().String(), ":", 2)[1])
			break
		}
	}

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

	if d.config.KeepaliveTime > 0 {
		// set linger 0 and tcp keepalive setting between client connection
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
		tcpConn.SetLinger(0)

		// set deadline
		tcpConn.SetDeadline(time.Now().Add(time.Duration(d.config.KeepaliveTime) * time.Second))
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
	tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	// set deadline
	tcpConn.SetDeadline(time.Now().Add(time.Duration(d.config.KeepaliveTime) * time.Second))

	return nil
}

// send data until got EOF
func (d *dataListener) dataTransfer(reader net.Conn, writer net.Conn) error {
	var err error

	buffer := make([]byte, DATA_TRANSFER_BUFFER_SIZE)
	if _, err := io.CopyBuffer(writer, reader, buffer); err != nil {
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

// get listen port
// return 0 means random port
func (d *dataListener) getListenPort() int {
	if len(d.config.DataPortRange) > 0 {
		portRange := strings.Split(d.config.DataPortRange, "-")

		if len(portRange) != 2 {
			d.log.debug("The port range config wrong. choose random port")
			return 0
		}

		min, _ := strconv.Atoi(strings.TrimSpace(portRange[0]))
		max, _ := strconv.Atoi(strings.TrimSpace(portRange[1]))

		r := max - min

		if r <= 0 {
			r = 1
		}

		return min + rand.Intn(r)
	}

	// let system automatically chose one port
	return 0
}
