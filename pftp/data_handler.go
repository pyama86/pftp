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
	DATA_CONNECTION_TIMEOUT   = 30
)

type dataHandler struct {
	clientConn   connector
	originConn   connector
	config       *config
	log          *logger
	waitResponse chan bool
	connected    chan bool
}

type connector struct {
	listener    *net.TCPListener
	conn        net.Conn
	remoteIP    string
	remotePort  string
	localIP     string
	localPort   string
	needsListen bool
	isClient    bool
	mode        string
	connDone    chan error
}

// Make listener for data connection
func newDataHandler(config *config, log *logger, clientConn net.Conn, originConn net.Conn, mode string) (*dataHandler, error) {
	var err error

	d := &dataHandler{
		originConn: connector{
			listener:    nil,
			conn:        nil,
			needsListen: false,
			isClient:    false,
			mode:        config.TransferMode,
			connDone:    make(chan error),
		},
		clientConn: connector{
			listener:    nil,
			conn:        nil,
			needsListen: false,
			isClient:    true,
			mode:        mode,
			connDone:    make(chan error),
		},
		config:       config,
		log:          log,
		waitResponse: make(chan bool),
		connected:    make(chan bool),
	}

	if originConn != nil {
		d.originConn.remoteIP, _, _ = net.SplitHostPort(originConn.RemoteAddr().String())
		d.originConn.localIP, d.originConn.localPort, _ = net.SplitHostPort(originConn.LocalAddr().String())
	}

	if clientConn != nil {
		d.clientConn.remoteIP, _, _ = net.SplitHostPort(clientConn.RemoteAddr().String())
		d.clientConn.localIP, d.clientConn.localPort, _ = net.SplitHostPort(clientConn.LocalAddr().String())
	}

	// When connections are nil, will not set listener
	if clientConn == nil || originConn == nil {
		return d, nil
	}

	if d.clientConn.mode != "PORT" {
		d.clientConn.listener, err = d.setNewListener()
		if err != nil {
			return nil, err
		}
		d.clientConn.needsListen = true
	}

	if d.originConn.mode == "PORT" || (d.originConn.mode == "CLIENT" && d.clientConn.mode == "PORT") {
		d.originConn.listener, err = d.setNewListener()
		if err != nil {
			return nil, err
		}
		d.originConn.needsListen = true
	}

	return d, nil
}

// get listen port
func getListenPort(dataPortRange string) string {
	// random port select
	if len(dataPortRange) == 0 {
		return ""
	}

	portRange := strings.Split(dataPortRange, "-")
	min, _ := strconv.Atoi(strings.TrimSpace(portRange[0]))
	max, _ := strconv.Atoi(strings.TrimSpace(portRange[1]))

	// return min port number
	if min == max {
		return strconv.Itoa(min)
	}

	// random select in min - max range
	return strconv.Itoa(min + rand.Intn(max-min))
}

// parse PASV IP and Port from server response
func parseLineToAddr(line string) (string, string, error) {
	addr := strings.Split(line, ",")

	if len(addr) != 6 {
		return "", "", fmt.Errorf("invalid data address")
	}

	// Get IP string from line
	ip := strings.Join(addr[0:4], ".")

	// get port number from line
	port1, _ := strconv.Atoi(addr[4])
	port2, _ := strconv.Atoi(addr[5])

	port := (port1 << 8) + port2

	// check IP and Port is valid
	if net.ParseIP(ip) == nil {
		return "", "", fmt.Errorf("invalid data address")
	}

	if port <= 0 || port > 65535 {
		return "", "", fmt.Errorf("invalid data address")
	}

	return ip, strconv.Itoa(port), nil
}

func (d *dataHandler) setNewListener() (*net.TCPListener, error) {
	var listener *net.TCPListener
	var lAddr *net.TCPAddr
	var err error
	// reallocate listener port when selected port is busy until LISTEN_TIMEOUT
	counter := 0
	for {
		counter++

		lAddr, err = net.ResolveTCPAddr("tcp", ":"+getListenPort(d.config.DataPortRange))
		if err != nil {
			d.log.err("cannot resolve TCPAddr")
			return nil, err
		}

		if listener, err = net.ListenTCP("tcp4", lAddr); err != nil {
			if counter > DATA_CONNECTION_TIMEOUT {
				d.log.err("cannot set listener")

				return nil, err
			}

			d.log.debug("cannot use choosen port. try to select another port after 1 second... (%d/%d)", counter, DATA_CONNECTION_TIMEOUT)

			time.Sleep(time.Duration(1) * time.Second)
			continue

		} else {
			// set listener timeout
			listener.SetDeadline(time.Now().Add(time.Duration(DATA_CONNECTION_TIMEOUT) * time.Second))

			break
		}
	}

	return listener, nil
}

func (d *dataHandler) Close() {
	// close each net.Conn absolutely(for end goroutine)
	if d.clientConn.conn != nil {
		d.clientConn.conn.Close()
	}
	if d.originConn.conn != nil {
		d.originConn.conn.Close()
	}

	// close listener
	if d.clientConn.listener != nil {
		d.clientConn.listener.Close()
	}
	if d.originConn.listener != nil {
		d.originConn.listener.Close()
	}
}

// Make listener for data connection
func (d *dataHandler) StartDataTransfer() error {
	var err error

	eg := errgroup.Group{}

	defer func() {
		d.Close()
	}()

	// if client connect mode is not PORT, connect to origin before connect to client
	if d.clientConn.needsListen {
		// if client connect mode is PASV or EPSV, start client listener
		go d.listenClient()
	} else {
		go d.connectToClient()
	}
	// if origin connect mode is not PORT, connect to origin before connect to client
	if d.originConn.needsListen {
		// start origin listener
		go d.listenOrigin()
	} else {
		// connect immediatly
		go d.connectToOrigin()
	}

	// wait until connect end
	isDonn := <-d.clientConn.connDone
	if isDonn != nil {
		err := fmt.Errorf("data connection is not fully established")
		d.log.err(err.Error())
		return err
	}

	// client to origin
	eg.Go(func() error { return d.dataTransfer(d.clientConn.conn, d.originConn.conn) })

	// origin to client
	eg.Go(func() error { return d.dataTransfer(d.originConn.conn, d.clientConn.conn) })

	// wait until data transfer goroutine end
	if err = eg.Wait(); err != nil {
		d.log.err(err.Error())
	}

	d.log.debug("proxy data channel disconnected")

	return nil
}

func (d *dataHandler) listenOrigin() error {
	conn, err := d.originConn.listener.AcceptTCP()
	if err != nil || conn == nil {
		d.log.err("error on origin connection listen: %v, %s", conn, err.Error())
		safeSetChanel(d.originConn.connDone, err)
		return err
	}

	d.originConn.listener.Close()
	d.originConn.listener = nil

	d.log.debug("origin connected from %s", conn.RemoteAddr().String())

	// set linger 0 and tcp keepalive setting between client connection
	if d.config.KeepaliveTime > 0 {
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
		conn.SetLinger(0)
	}

	d.originConn.conn = conn
	safeSetChanel(d.originConn.connDone, nil)

	return nil
}

func (d *dataHandler) listenClient() error {
	isDonn := <-d.originConn.connDone
	if isDonn != nil {
		d.log.err("origin connections is failed")
		return fmt.Errorf("origin connections is failed")
	}

	conn, err := d.clientConn.listener.AcceptTCP()
	if err != nil || conn == nil {
		d.log.err("error on client connection listen: %v, %s", conn, err.Error())
		safeSetChanel(d.clientConn.connDone, err)
		return err
	}

	d.clientConn.listener.Close()
	d.clientConn.listener = nil

	d.log.debug("client connected from %s", conn.RemoteAddr().String())

	// set linger 0 and tcp keepalive setting between client connection
	if d.config.KeepaliveTime > 0 {
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
		conn.SetLinger(0)
	}

	d.clientConn.conn = conn
	safeSetChanel(d.clientConn.connDone, nil)

	return nil
}

func (d *dataHandler) connectToOrigin() error {
	conn, err := net.Dial(
		"tcp",
		net.JoinHostPort(d.originConn.remoteIP, d.originConn.remotePort))
	if err != nil || conn == nil {
		d.log.err("connect to origin error: %v, %s", conn, err.Error())
		safeSetChanel(d.originConn.connDone, err)
		return err
	}

	d.log.debug("connected to origin %s", conn.RemoteAddr().String())

	// set linger 0 and tcp keepalive setting between client connection
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	// d.originConn.conn = tcpConn
	d.originConn.conn = conn

	safeSetChanel(d.originConn.connDone, nil)

	return nil
}

func (d *dataHandler) connectToClient() error {
	isDonn := <-d.originConn.connDone
	if isDonn != nil {
		d.log.err("origin connections is failed")
		return fmt.Errorf("origin connections is failed")
	}

	// when connect to client(use active mode), dial to client use port 20 only
	lAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", "20"))
	if err != nil {
		d.log.err("cannot resolve local address")
		safeSetChanel(d.clientConn.connDone, err)
		return err
	}
	// set port reuse and local address
	netDialer := net.Dialer{
		Control:   setReuseIPPort,
		LocalAddr: lAddr,
		Deadline:  time.Now().Add(time.Duration(DATA_CONNECTION_TIMEOUT) * time.Second),
	}

	conn, err := netDialer.Dial("tcp", net.JoinHostPort(d.clientConn.remoteIP, d.clientConn.remotePort))
	if err != nil || conn == nil {
		d.log.err("connect to client error: %v, %s", conn, err.Error())
		safeSetChanel(d.clientConn.connDone, err)
		return err
	}

	d.log.debug("connect to client %s", conn.RemoteAddr().String())

	// set linger 0 and tcp keepalive setting between client connection
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	d.clientConn.conn = tcpConn

	safeSetChanel(d.clientConn.connDone, nil)
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

func (d *dataHandler) parsePORTcommand(line string) error {
	// PORT command format : "PORT h1,h2,h3,h4,p1,p2\r\n"
	var err error

	d.clientConn.remoteIP, d.clientConn.remotePort, err = parseLineToAddr(strings.Split(strings.Trim(line, "\r\n"), " ")[1])

	return err
}

func (d *dataHandler) parsePASVresponse(line string) error {
	// PASV response format : "227 Entering Passive Mode (h1,h2,h3,h4,p1,p2).\r\n"
	var err error

	startIndex := strings.Index(line, "(")
	endIndex := strings.LastIndex(line, ")")

	if startIndex == -1 || endIndex == -1 {
		return errors.New("invalid data address")
	}

	d.originConn.remoteIP, d.originConn.remotePort, err = parseLineToAddr(line[startIndex+1 : endIndex])

	return err
}

func (d *dataHandler) parseEPSVresponse(line string) error {
	// EPSV response format : "229 Entering Extended Passive Mode (|||port|)\r\n"
	startIndex := strings.Index(line, "(")
	endIndex := strings.LastIndex(line, ")")

	if startIndex == -1 || endIndex == -1 {
		return errors.New("invalid data address")
	}

	// get port and verify it
	originPort := strings.Trim(line[startIndex+1:endIndex], "|")
	port, _ := strconv.Atoi(originPort)
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid data address")
	}

	d.originConn.remotePort = originPort

	return nil
}
