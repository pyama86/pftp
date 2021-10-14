package pftp

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
	"github.com/tevino/abool"
	"golang.org/x/sync/errgroup"
)

const (
	uploadStream   = "upload"
	downloadStream = "download"
	abortStream    = "abort"
)

type dataHandler struct {
	clientConn         connector
	originConn         connector
	config             *config
	log                *logger
	tlsDataSet         *tlsDataSet
	needTLSForTransfer *abool.AtomicBool
	inDataTransfer     *abool.AtomicBool
	closed             bool
	mutex              *sync.Mutex
}

type connector struct {
	listener          *net.TCPListener
	communicationConn net.Conn
	dataConn          net.Conn
	originalRemoteIP  string
	remoteIP          string
	remotePort        string
	localIP           string
	localPort         string
	needsListen       bool
	isClient          bool
	mode              string
}

// Make listener for data connection
func newDataHandler(config *config, log *logger, clientConn net.Conn, originConn net.Conn, mode string, tlsDataSet *tlsDataSet, transferOverTLS *abool.AtomicBool, inDataTransfer *abool.AtomicBool) (*dataHandler, error) {
	var err error

	d := &dataHandler{
		originConn: connector{
			listener:          nil,
			communicationConn: originConn,
			dataConn:          nil,
			needsListen:       false,
			isClient:          false,
			mode:              config.TransferMode,
		},
		clientConn: connector{
			listener:          nil,
			communicationConn: clientConn,
			dataConn:          nil,
			needsListen:       false,
			isClient:          true,
			mode:              mode,
		},
		config:             config,
		log:                log,
		inDataTransfer:     inDataTransfer,
		tlsDataSet:         tlsDataSet,
		needTLSForTransfer: transferOverTLS,
		closed:             false,
		mutex:              &sync.Mutex{},
	}

	if d.originConn.communicationConn != nil {
		d.originConn.originalRemoteIP, _, _ = net.SplitHostPort(originConn.RemoteAddr().String())
		d.originConn.localIP, d.originConn.localPort, _ = net.SplitHostPort(originConn.LocalAddr().String())
	}

	if d.clientConn.communicationConn != nil {
		d.clientConn.originalRemoteIP, _, _ = net.SplitHostPort(clientConn.RemoteAddr().String())
		d.clientConn.localIP, d.clientConn.localPort, _ = net.SplitHostPort(clientConn.LocalAddr().String())
	}

	// When connections are nil, will not set listener
	if clientConn == nil || originConn == nil {
		return d, nil
	}

	// init client connection
	if checkNeedListen(d.clientConn.mode, d.originConn.mode, true) {
		d.clientConn.listener, err = d.setNewListener()
		if err != nil {
			connectionCloser(d, d.log)

			return nil, err
		}
		d.clientConn.needsListen = true
	}

	// init origin connection
	if checkNeedListen(d.clientConn.mode, d.originConn.mode, false) {
		d.originConn.listener, err = d.setNewListener()
		if err != nil {
			connectionCloser(d, d.log)

			return nil, err
		}
		d.originConn.needsListen = true
	}

	return d, nil
}

// check needs to open listener
func checkNeedListen(clientMode string, originMode string, isClient bool) bool {
	if isClient {
		if clientMode == "PASV" || clientMode == "EPSV" {
			return true
		}
	} else {
		if originMode == "PORT" || originMode == "EPRT" {
			return true
		} else if originMode == "CLIENT" {
			if clientMode == "PORT" || clientMode == "EPRT" {
				return true
			}
		}
	}

	return false
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

// assign listen port create listener
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

		if listener, err = net.ListenTCP("tcp", lAddr); err != nil {
			if counter > connectionTimeout {
				d.log.err("cannot set listener")

				return nil, err
			}

			d.log.debug("cannot use choosen port. try to select another port after 1 second... (%d/%d)", counter, connectionTimeout)

			time.Sleep(time.Duration(1) * time.Second)
			continue

		} else {
			d.log.debug("data listen port selected: '%s'", lAddr.String())
			break
		}
	}

	return listener, nil
}

// close all connection and listener
func (d *dataHandler) Close() error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// return nil when handler already closed
	if d.closed {
		return nil
	}

	lastErr := error(nil)

	// close net.Conn
	if d.clientConn.dataConn != nil {
		if err := d.clientConn.dataConn.Close(); err != nil {
			if !strings.Contains(err.Error(), alreadyClosedMsg) {
				lastErr = fmt.Errorf("client data connection close error: %s", err.Error())
			}
		}
		d.clientConn.dataConn = nil
	}
	if d.originConn.dataConn != nil {
		if err := d.originConn.dataConn.Close(); err != nil {
			if !strings.Contains(err.Error(), alreadyClosedMsg) {
				lastErr = fmt.Errorf("origin data connection close error: %s", err.Error())
			}
		}
		d.originConn.dataConn = nil
	}

	// close listener
	if d.clientConn.listener != nil {
		if err := d.clientConn.listener.Close(); err != nil {
			if !strings.Contains(err.Error(), alreadyClosedMsg) {
				lastErr = fmt.Errorf("client data listener close error: %s", err.Error())
			}
		}
		d.clientConn.listener = nil
	}
	if d.originConn.listener != nil {
		if err := d.originConn.listener.Close(); err != nil {
			if !strings.Contains(err.Error(), alreadyClosedMsg) {
				lastErr = fmt.Errorf("origin data listener close error: %s", err.Error())
			}
		}
		d.originConn.listener = nil
	}

	d.closed = true
	d.inDataTransfer.UnSet()

	d.log.debug("proxy data channel disconnected")

	return lastErr
}

// return current handler closed state
func (d *dataHandler) isClosed() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.closed
}

// return true when handler start transfer progress
func (d *dataHandler) isStarted() bool {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.inDataTransfer.IsSet()
}

// Make listener for data connection
func (d *dataHandler) StartDataTransfer(direction string) error {
	var err error

	defer connectionCloser(d, d.log)

	eg := errgroup.Group{}

	// make data connection (client first)
	clientConnected := make(chan error)
	eg.Go(func() error {
		if err := d.clientListenOrDial(clientConnected); err != nil {
			connectionCloser(d, d.log)
			return err
		}

		return nil
	})
	eg.Go(func() error {
		if err := d.originListenOrDial(clientConnected); err != nil {
			connectionCloser(d, d.log)
			return err
		}

		return nil
	})

	// wait until copy goroutine end
	if err := eg.Wait(); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			d.log.debug("data connection aborted by EOF")
		} else {
			d.log.err("data connection creation failed: %s", err.Error())
		}

		return err
	}

	d.log.debug("start %s data transfer", direction)

	// do not timeout communication connection during data transfer
	d.clientConn.communicationConn.SetDeadline(time.Time{})
	d.originConn.communicationConn.SetDeadline(time.Time{})

	if err := d.run(); err != nil {
		if !strings.Contains(err.Error(), alreadyClosedMsg) {
			d.log.err("got error on %s data transfer: %s", direction, err.Error())
		}
	} else {
		d.log.debug("%s data transfer finished", direction)
	}

	// set timeout to each connection
	d.clientConn.communicationConn.SetDeadline(time.Now().Add(time.Duration(d.config.IdleTimeout) * time.Second))
	d.originConn.communicationConn.SetDeadline(time.Now().Add(time.Duration(d.config.ProxyTimeout) * time.Second))

	return err
}

// make client connection
func (d *dataHandler) clientListenOrDial(clientConnected chan error) error {
	// if client connect needs listen, open listener
	if d.clientConn.needsListen {
		d.mutex.Lock()
		if d.closed {
			d.mutex.Unlock()
			return errors.New("abort: data handler already closed")
		}
		listener := d.clientConn.listener
		d.mutex.Unlock()

		// set listener timeout
		listener.SetDeadline(time.Now().Add(time.Duration(connectionTimeout) * time.Second))

		conn, err := listener.AcceptTCP()
		clientConnected <- err
		if err != nil {
			return err
		}

		d.log.debug("client connected from %s", conn.RemoteAddr().String())
		d.log.debug("close listener %s", listener.Addr().String())

		// release listener for reuse
		if err := listener.Close(); err != nil {
			if !strings.Contains(err.Error(), alreadyClosedMsg) {
				d.log.err("cannot close client data listener: %s", err.Error())
			}
		}

		// set linger 0 and tcp keepalive setting between client connection
		if d.config.KeepaliveTime > 0 {
			conn.SetKeepAlive(true)
			conn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
			conn.SetLinger(0)
		}

		d.clientConn.dataConn = conn
	} else {
		var conn net.Conn
		var err error

		// when connect to client(use active mode), dial to client use port 20 only
		lAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", "20"))
		if err != nil {
			return fmt.Errorf("cannot resolve local address: %s", err.Error())
		}
		// set port reuse and local address
		netDialer := net.Dialer{
			Control:   setReuseIPPort,
			LocalAddr: lAddr,
			Deadline:  time.Now().Add(time.Duration(connectionTimeout) * time.Second),
		}

		conn, err = netDialer.Dial("tcp", net.JoinHostPort(d.clientConn.remoteIP, d.clientConn.remotePort))
		clientConnected <- err
		if err != nil {
			return fmt.Errorf("cannot connect to client data address: %v, %s", conn, err.Error())
		}

		// set linger 0 and tcp keepalive setting between client connection
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
		tcpConn.SetLinger(0)

		d.clientConn.dataConn = tcpConn
	}

	if d.needTLSForTransfer.IsSet() {
		if d.tlsDataSet.forClient.getTLSConfig() == nil {
			return errors.New("cannot get client TLS config for data transfer. abort data transfer")
		}

		d.mutex.Lock()
		if d.closed {
			d.mutex.Unlock()
			return errors.New("abort: data handler already closed")
		}
		dataConn := d.clientConn.dataConn
		d.mutex.Unlock()

		tlsConn := tls.Server(dataConn, d.tlsDataSet.forClient.getTLSConfig())
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("TLS client data connection handshake got error: %v", err)
		}
		d.log.debug("TLS data connection with client has set. TLS protocol version: %s and Cipher Suite: %s. (resumed?: %v)", getTLSProtocolName(tlsConn.ConnectionState().Version), tls.CipherSuiteName(tlsConn.ConnectionState().CipherSuite), tlsConn.ConnectionState().DidResume)

		d.clientConn.dataConn = tlsConn
	}

	// set transfer timeout to data connection
	d.mutex.Lock()
	if !d.closed {
		d.clientConn.dataConn.SetDeadline(time.Now().Add(time.Duration(d.config.TransferTimeout) * time.Second))
	}
	d.mutex.Unlock()

	return nil
}

// make origin connection
func (d *dataHandler) originListenOrDial(clientConnected chan error) error {
	// if client data connection got error, abort origin connection too
	if <-clientConnected != nil {
		return nil
	}

	// if origin connect needs listen, open listener
	if d.originConn.needsListen {
		d.mutex.Lock()
		if d.closed {
			d.mutex.Unlock()
			return errors.New("abort: data handler already closed")
		}
		listener := d.originConn.listener
		d.mutex.Unlock()

		// set listener timeout
		listener.SetDeadline(time.Now().Add(time.Duration(connectionTimeout) * time.Second))

		conn, err := listener.AcceptTCP()
		if err != nil {
			return err
		}

		d.log.debug("origin connected from %s", conn.RemoteAddr().String())
		d.log.debug("close listener %s", listener.Addr().String())

		// release listener for reuse
		if err := listener.Close(); err != nil {
			if !strings.Contains(err.Error(), alreadyClosedMsg) {
				d.log.err("cannot close origin data listener: %s", err.Error())
			}
		}

		// set linger 0 and tcp keepalive setting between origin connection
		if d.config.KeepaliveTime > 0 {
			conn.SetKeepAlive(true)
			conn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
			conn.SetLinger(0)
		}

		d.originConn.dataConn = conn

	} else {
		var conn net.Conn
		var err error

		conn, err = net.DialTimeout(
			"tcp",
			net.JoinHostPort(d.originConn.remoteIP, d.originConn.remotePort),
			time.Duration(connectionTimeout)*time.Second,
		)
		if err != nil {
			return fmt.Errorf("cannot connect to origin data address: %v, %s", conn, err.Error())
		}

		d.log.debug("connected to origin %s", conn.RemoteAddr().String())

		// set linger 0 and tcp keepalive setting between origin connection
		tcpConn := conn.(*net.TCPConn)
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Duration(d.config.KeepaliveTime) * time.Second)
		tcpConn.SetLinger(0)

		d.originConn.dataConn = tcpConn

		if d.config.ProxyProtocol {
			sourcePortInt := 4242 // XXX TODO use the client Port

			destinationPortInt, err := strconv.Atoi(d.originConn.remotePort)
			if err != nil {
				return err
			}

			transportProtocol := proxyproto.TCPv4
			if strings.Count(d.clientConn.localIP, ":") > 0 {
				transportProtocol = proxyproto.TCPv6
			}

			proxyProtocolHeader := proxyproto.Header{
				Version:           byte(1),
				Command:           proxyproto.PROXY,
				TransportProtocol: transportProtocol,
				SourceAddr:        &net.TCPAddr{IP: net.ParseIP(d.clientConn.localIP), Port: sourcePortInt},
				DestinationAddr:   &net.TCPAddr{IP: net.ParseIP(d.originConn.remoteIP), Port: destinationPortInt},
			}

			_, err = proxyProtocolHeader.WriteTo(tcpConn)
			return err
		}
	}

	// set TLS session.
	if d.needTLSForTransfer.IsSet() {
		if d.tlsDataSet.forOrigin.getTLSConfig() == nil {
			return errors.New("cannot get origin TLS config for data transfer. abort data transfer")
		}

		d.mutex.Lock()
		if d.closed {
			d.mutex.Unlock()
			return errors.New("abort: data handler already closed")
		}
		dataConn := d.originConn.dataConn
		d.mutex.Unlock()

		tlsConn := tls.Client(dataConn, d.tlsDataSet.forOrigin.getTLSConfig())
		if err := tlsConn.Handshake(); err != nil {
			return fmt.Errorf("TLS origin data connection handshake got error: %v", err)
		}
		d.log.debug("TLS data connection with origin has set. TLS protocol version: %s and Cipher Suite: %s. (resumed?: %v)", getTLSProtocolName(tlsConn.ConnectionState().Version), tls.CipherSuiteName(tlsConn.ConnectionState().CipherSuite), tlsConn.ConnectionState().DidResume)

		d.originConn.dataConn = tlsConn
	}

	// set transfer timeout to data connection
	d.mutex.Lock()
	if !d.closed {
		d.originConn.dataConn.SetDeadline(time.Now().Add(time.Duration(d.config.TransferTimeout) * time.Second))
	}
	d.mutex.Unlock()

	return nil
}

// make full duplex connection between client and origin sockets
func (d *dataHandler) run() error {
	eg := errgroup.Group{}

	// origin to client
	eg.Go(func() error {
		return d.copyPackets(d.clientConn.dataConn, d.originConn.dataConn, d.config.TransferTimeout)
	})
	// client to origin
	eg.Go(func() error {
		return d.copyPackets(d.originConn.dataConn, d.clientConn.dataConn, d.config.TransferTimeout)
	})

	// wait until copy goroutine end
	err := eg.Wait()

	return err
}

// send src packet to dst.
// replace io.Copy function to manual coding because io.Copy
// function can not increase src conn's deadline per each read.
func (d *dataHandler) copyPackets(dst net.Conn, src net.Conn, timeout int) error {
	lastErr := error(nil)
	buff := make([]byte, bufferSize)

	for {
		// check about aborted from outside of handler
		if d.isClosed() {
			break
		}

		n, err := src.Read(buff)
		if n > 0 {
			// stop coping when failed to write dst socket
			if _, err := dst.Write(buff[:n]); err != nil {
				dst.Close()
				break
			}
			// increase data transfer timeout
			src.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		}
		if err != nil {
			if err == io.EOF {
				// got EOF from src, send EOF to dst
				lastErr = sendEOF(dst)
			} else {
				lastErr = err
			}

			break
		}
	}

	return lastErr
}

// parse port comand line (active data conn)
func (d *dataHandler) parsePORTcommand(line string) error {
	// PORT command format : "PORT h1,h2,h3,h4,p1,p2\r\n"
	var err error

	d.clientConn.remoteIP, d.clientConn.remotePort, err = parseLineToAddr(strings.Split(strings.Trim(line, "\r\n"), " ")[1])

	// if received ip is not public IP, ignore it
	if !isPublicIP(net.ParseIP(d.clientConn.remoteIP)) {
		d.clientConn.remoteIP = d.clientConn.originalRemoteIP
	}

	return err
}

// parse eprt comand line (active data conn)
func (d *dataHandler) parseEPRTcommand(line string) error {
	// EPRT command format
	// - IPv4 : "EPRT |1|h1.h2.h3.h4|port|\r\n"
	// - IPv6 : "EPRT |2|h1::h2:h3:h4:h5|port|\r\n"
	var err error

	d.clientConn.remoteIP, d.clientConn.remotePort, err = parseEPRTtoAddr(strings.Split(strings.Trim(line, "\r\n"), " ")[1])

	// if received ip is not public IP, ignore it
	if !isPublicIP(net.ParseIP(d.clientConn.remoteIP)) {
		d.clientConn.remoteIP = d.clientConn.originalRemoteIP
	}

	return err
}

// parse pasv comand line (passive data conn)
func (d *dataHandler) parsePASVresponse(line string) error {
	// PASV response format : "227 Entering Passive Mode (h1,h2,h3,h4,p1,p2).\r\n"
	var err error

	startIndex := strings.Index(line, "(")
	endIndex := strings.LastIndex(line, ")")

	if startIndex == -1 || endIndex == -1 {
		return errors.New("invalid data address")
	}

	d.originConn.remoteIP, d.originConn.remotePort, err = parseLineToAddr(line[startIndex+1 : endIndex])

	// if received ip is not public IP, ignore it
	if !isPublicIP(net.ParseIP(d.originConn.remoteIP)) || d.config.IgnorePassiveIP {
		d.originConn.remoteIP = d.originConn.originalRemoteIP
	}

	return err
}

// parse epsv comand line (passive data conn)
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

// parse IP and Port from line
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

// parse EPRT command from client
func parseEPRTtoAddr(line string) (string, string, error) {
	addr := strings.Split(line, "|")

	if len(addr) != 5 {
		return "", "", fmt.Errorf("invalid data address")
	}

	netProtocol := addr[1]
	IP := addr[2]

	// check port is valid
	port := addr[3]
	if integerPort, err := strconv.Atoi(port); err != nil {
		return "", "", fmt.Errorf("invalid data address")
	} else if integerPort <= 0 || integerPort > 65535 {
		return "", "", fmt.Errorf("invalid data address")
	}

	switch netProtocol {
	case "1", "2":
		// use protocol 1 means IPv4. 2 means IPv6
		// net.ParseIP for validate IP
		if net.ParseIP(IP) == nil {
			return "", "", fmt.Errorf("invalid data address")
		}
	default:
		// wrong network protocol
		return "", "", fmt.Errorf("unknown network protocol")
	}

	return IP, port, nil
}

// check IP is public
// ** private IP range **
// Class       Starting IPAddress     Ending IP Address    # Host counts
// A           10.0.0.0               10.255.255.255       16,777,216
// B           172.16.0.0             172.31.255.255       1,048,576
// C           192.168.0.0            192.168.255.255      65,536
// Link-local  169.254.0.0            169.254.255.255      65,536
// Local       127.0.0.0              127.255.255.255      16777216
func isPublicIP(IP net.IP) bool {
	if IP.IsLoopback() || IP.IsLinkLocalMulticast() || IP.IsLinkLocalUnicast() {
		return false
	}
	if ip4 := IP.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return false
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return false
		case ip4[0] == 192 && ip4[1] == 168:
			return false
		default:
			return true
		}
	}
	return false
}
