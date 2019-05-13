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

type dataHandler struct {
	mode         string
	listener     *net.TCPListener
	txConn       net.Conn
	rxConn       net.Conn
	remoteAddr   string
	remoteIP     string
	remotePort   int
	receivedIP   string
	listenerPort int
	config       *config
	log          *logger
}

// Make listener for data connection
func newDataHandler(line string, receivedIP string, config *config, log *logger, mode string) (string, int, error) {
	var err error
	var lAddr *net.TCPAddr

	d := &dataHandler{
		mode:       mode,
		receivedIP: receivedIP,
		listener:   nil,
		txConn:     nil,
		rxConn:     nil,
		config:     config,
		log:        log,
	}

	startIndex := strings.Index(line, "(")
	endIndex := strings.LastIndex(line, ")")

	switch d.mode {
	case "PASV":
		// PASV response format : "227 Entering Passive Mode (h1,h2,h3,h4,p1,p2)."
		if startIndex == -1 || endIndex == -1 {
			return "", -1, errors.New("invalid data address")
		}

		if d.remoteIP, d.remotePort, err = parseToAddr(line[startIndex+1 : endIndex]); err != nil {
			return "", -1, err
		}
	case "EPSV":
		// EPSV response format : "229 Entering Extended Passive Mode (|||port|)"
		if startIndex == -1 || endIndex == -1 {
			return "", -1, errors.New("invalid data address")
		}

		d.remoteIP = d.receivedIP

		// get port and verify it
		originPort := strings.Trim(line[startIndex+1:endIndex], "|")
		port, _ := strconv.Atoi(originPort)
		if port <= 0 || port > 65535 {
			return "", -1, fmt.Errorf("invalid data address")
		}

		d.remotePort = port
	case "PORT":
		// PORT command format : "PORT h1,h2,h3,h4,p1,p2\r\n"
		if startIndex != -1 || endIndex != -1 {
			return "", -1, errors.New("invalid data address")
		}

		if d.remoteIP, d.remotePort, err = parseToAddr(strings.Split(strings.Trim(line, "\r\n"), " ")[1]); err != nil {
			return "", -1, err
		}
	}

	// if config == nil, does not open open and start listener
	if config == nil {
		return d.receivedIP, d.listenerPort, nil
	}

	// reallocate listener port when selected port is busy until LISTEN_TIMEOUT
	counter := 0
	for {
		lAddr, err = net.ResolveTCPAddr("tcp", net.JoinHostPort(d.receivedIP, d.getListenPort()))
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

	// start listener & listener timer
	go d.startDataListener()

	return d.receivedIP, d.listenerPort, nil
}

// Make listener for data connection
func (d *dataHandler) startDataListener() error {
	var err error

	eg := errgroup.Group{}

	defer func() {
		// close each net.Conn absolutely(for end goroutine)
		if d.rxConn != nil {
			d.rxConn.Close()
		}

		if d.txConn != nil {
			d.txConn.Close()
		}

		// close listener
		if d.listener != nil {
			d.listener.Close()
		}
	}()

	conn, err := d.listener.AcceptTCP()
	if err != nil {
		d.log.err("listen error : %s", err.Error())
		return err
	}
	// close listener immediatly
	d.listener.Close()
	d.listener = nil

	d.log.info("Data channel connected from %s", conn.RemoteAddr().String())

	// set linger 0 and tcp keepalive setting between client connection
	if d.config.KeepaliveTime > 0 {
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
		conn.SetLinger(0)
	}

	d.rxConn = conn

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
func (d *dataHandler) connectToRemoteDataChan() error {
	var err error

	rAddr := net.JoinHostPort(d.remoteIP, strconv.Itoa(d.remotePort))

	var conn net.Conn
	switch d.mode {
	case "PORT":
		// if use active mode, dial to client only by port 20
		lAddr, err := net.ResolveTCPAddr("tcp", ":20")
		if err != nil {
			d.log.err("cannot resolve TCPAddr")
			return err
		}

		// set port reuse and local addr( :20 )
		d := net.Dialer{
			Control:   setReuseIPPort,
			LocalAddr: lAddr,
		}
		conn, err = d.Dial("tcp", rAddr)
	default:
		conn, err = net.Dial("tcp", rAddr)
	}
	if err != nil {
		return err
	}

	// set linger 0 and tcp keepalive setting between client connection
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	d.txConn = tcpConn

	return nil
}

// send data until got EOF
func (d *dataHandler) dataTransfer(reader net.Conn, writer net.Conn) error {
	var err error

	buffer := make([]byte, DATA_TRANSFER_BUFFER_SIZE)
	if _, err := io.CopyBuffer(writer, reader, buffer); err != nil {
		err = fmt.Errorf("got error on data transfer: %s", err.Error())
	}

	// send EOF to writer
	sendEOF(writer)

	return err
}

// parse PASV IP and Port from server response
func parseToAddr(line string) (string, int, error) {
	addr := strings.Split(line, ",")

	if len(addr) != 6 {
		return "", -1, fmt.Errorf("invalid data address")
	}

	// Get IP string from line
	ip := strings.Join(addr[0:4], ".")

	// get port number from line
	port1, _ := strconv.Atoi(addr[4])
	port2, _ := strconv.Atoi(addr[5])

	port := (port1 << 8) + port2

	// check IP and Port is valid
	if net.ParseIP(ip) == nil {
		return "", -1, fmt.Errorf("invalid data address")
	}

	if port <= 0 || port > 65535 {
		return "", -1, fmt.Errorf("invalid data address")
	}

	return ip, port, nil
}

// get listen port
func (d *dataHandler) getListenPort() string {
	// random port select
	if len(d.config.DataPortRange) == 0 {
		return ""
	}

	portRange := strings.Split(d.config.DataPortRange, "-")
	min, _ := strconv.Atoi(strings.TrimSpace(portRange[0]))
	max, _ := strconv.Atoi(strings.TrimSpace(portRange[1]))

	// return min port number
	if min == max {
		return strconv.Itoa(min)
	}

	// random select in min - max range
	return strconv.Itoa(min + rand.Intn(max-min))
}
