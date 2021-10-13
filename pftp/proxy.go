package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
	"github.com/tevino/abool"
)

const (
	bufferSize             = 4096
	dataTransferBufferSize = 4096
	connectionTimeout      = 30
	secureCommand          = "PASS"
	alreadyClosedMsg       = "use of closed"
)

type proxyServer struct {
	clientReader          *bufio.Reader
	clientWriter          *bufio.Writer
	origin                net.Conn
	originReader          *bufio.Reader
	originWriter          *bufio.Writer
	tlsDatas              *tlsDataSet
	passThrough           bool
	mutex                 *sync.Mutex
	log                   *logger
	stopChan              chan struct{}
	stopChanDone          chan struct{}
	stop                  bool
	isLoggedin            bool
	welcomeMsg            string
	config                *config
	dataConnector         *dataHandler
	waitSwitching         chan bool
	inDataTransfer        *abool.AtomicBool
	isDataCommandResponse bool
}

type proxyServerConfig struct {
	clientReader   *bufio.Reader
	clientWriter   *bufio.Writer
	tlsDatas       *tlsDataSet
	originAddr     string
	mutex          *sync.Mutex
	log            *logger
	config         *config
	inDataTransfer *abool.AtomicBool
}

func newProxyServer(conf *proxyServerConfig) (*proxyServer, error) {
	c, err := net.DialTimeout("tcp",
		conf.originAddr,
		time.Duration(connectionTimeout)*time.Second)
	if err != nil {
		return nil, err
	}

	// set linger 0 and tcp keepalive setting between origin connection
	tcpConn := c.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(conf.config.KeepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	p := &proxyServer{
		clientReader:   conf.clientReader,
		clientWriter:   conf.clientWriter,
		originWriter:   bufio.NewWriter(c),
		originReader:   bufio.NewReader(c),
		origin:         tcpConn,
		tlsDatas:       conf.tlsDatas,
		passThrough:    true,
		mutex:          conf.mutex,
		log:            conf.log,
		stopChan:       make(chan struct{}),
		stopChanDone:   make(chan struct{}),
		welcomeMsg:     "220 " + conf.config.WelcomeMsg + "\r\n",
		isLoggedin:     false,
		config:         conf.config,
		waitSwitching:  make(chan bool),
		inDataTransfer: conf.inDataTransfer,
	}

	p.log.debug("new proxy from=%s to=%s", c.LocalAddr(), c.RemoteAddr())

	return p, err
}

// check command line validation
func (s *proxyServer) commandLineCheck(line string) (string, error) {
	// if first byte of command line is not alphabet, delete it until start with alphabet for avoid errors
	// FTP commands always start with alphabet.
	// ex) "\xff\xf4\xffABOR\r\n" -> "ABOR\r\n"
	for {
		// if line is empty, abort check
		if len(line) == 0 {
			return "", fmt.Errorf("aborted : wrong command line")
		}
		b := line[0]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
			break
		}
		line = line[1:]
	}

	// command line must contain CRLF only once in the end
	if !strings.HasSuffix(line, "\r\n") || strings.Count(line, "\r") != 1 || strings.Count(line, "\n") != 1 {
		s.log.debug("wrong command line. make line end by CRLF")

		// delete CR & LF characters from line (only allow to end of line "\r\n")
		line = strings.Replace(line, "\n", "", -1)
		line = strings.Replace(line, "\r", "", -1)

		// add write CRLF to end of line
		line += "\r\n"
	}

	return line, nil
}

func (s *proxyServer) sendToOrigin(line string) error {
	var err error

	// check command line and fix
	line, err = s.commandLineCheck(line)
	if err != nil {
		return err
	}

	s.commandLog(line)

	if _, err := s.originWriter.WriteString(line); err != nil {
		s.log.err("send to origin error: %s", err.Error())
		return err
	}
	if err := s.originWriter.Flush(); err != nil {
		return err
	}

	return nil
}

func (s *proxyServer) sendToClient(line string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, err := s.clientWriter.WriteString(line + "\r\n"); err != nil {
		return err
	}
	if err := s.clientWriter.Flush(); err != nil {
		return err
	}

	s.log.debug("send to client: %s", line)
	return nil
}

func (s *proxyServer) responseProxy() error {
	return s.startProxy()
}

func (s *proxyServer) suspend() {
	s.log.debug("suspend proxy")
	s.passThrough = false
}

func (s *proxyServer) unsuspend() {
	s.log.debug("unsuspend proxy")
	s.passThrough = true
}

// Close origin connection and check return
func (s *proxyServer) Close() error {
	if s.origin != nil {
		if err := s.origin.Close(); err != nil {
			return err
		}
	}

	if s.dataConnector != nil {
		s.DestroyDataHandler()
	}

	return nil
}

func (s *proxyServer) GetConn() net.Conn {
	return s.origin
}

// basically, this function never called during data transfer
// in progress, so block by chan is not necessary.
func (s *proxyServer) SetDataHandler(handler *dataHandler) {
	// cleanup previous data connector.
	if s.dataConnector != nil {
		s.DestroyDataHandler()
	}

	s.dataConnector = handler
}

// Destroy data handler
func (s *proxyServer) DestroyDataHandler() {
	if s.dataConnector != nil {
		connectionCloser(s.dataConnector, s.log)
	}
}

// return true when data handler is available now
func (s *proxyServer) isDataHandlerAvailable() bool {
	if s.dataConnector == nil {
		return false
	}

	return !s.dataConnector.isClosed()
}

// return true when data transfer in progress
func (s *proxyServer) isDataTransferStarted() bool {
	if s.dataConnector == nil {
		return false
	}

	return s.dataConnector.isStarted()
}

// return switch origin & user logged in state
func (s *proxyServer) isLoggedIn() bool {
	return s.isLoggedin
}

func (s *proxyServer) sendProxyHeader(clientAddr string, originAddr string) error {
	sourceAddr, sourcePort, err := net.SplitHostPort(clientAddr)
	if err != nil {
		return err
	}

	sourcePortInt, err := strconv.Atoi(sourcePort)
	if err != nil {
		return err
	}

	destinationAddr, destinationPort, err := net.SplitHostPort(originAddr)
	if err != nil {
		return err
	}

	destinationPortInt, err := strconv.Atoi(destinationPort)
	if err != nil {
		return err
	}

	// proxyProtocolHeader's DestinationAddress must be IP! not domain name
	hostIP, err := net.LookupIP(destinationAddr)
	if err != nil {
		return err
	}

	transportProtocol := proxyproto.TCPv4
	if strings.Count(sourceAddr, ":") > 0 {
		transportProtocol = proxyproto.TCPv6
	}

	proxyProtocolHeader := proxyproto.Header{
		Version:           byte(1),
		Command:           proxyproto.PROXY,
		TransportProtocol: transportProtocol,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP(sourceAddr), Port: sourcePortInt},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP(hostIP[0].String()), Port: destinationPortInt},
	}

	_, err = proxyProtocolHeader.WriteTo(s.origin)
	return err
}

// send command before login to origin
func (s *proxyServer) sendTLSCommand(previousTLSCommands []string) error {
	lastError := error(nil)

	for _, cmd := range previousTLSCommands {
		s.commandLog(cmd)
		if _, err := s.originWriter.WriteString(cmd); err != nil {
			return fmt.Errorf("failed to send AUTH command to origin")
		}
		if err := s.originWriter.Flush(); err != nil {
			return err
		}

		for {
			// Read response from new origin server
			str, err := s.originReader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to make TLS connection")
			}

			s.log.debug("response from origin: %s", strings.TrimSuffix(str, "\r\n"))

			if strings.Compare(strings.ToUpper(getCommand(cmd)[0]), "AUTH") == 0 {
				code := getCode(str)[0]
				if code != "234" {
					// when got 500 PROXY not understood, ignore it
					// this ignore setting for complex origins.
					// if some origins needs proxy protocol and some else is not,
					// pftp cannot support both in same time. So, pftp ignore the
					// 500 PROXY not understood then client can connect any servers.
					if s.config.ProxyProtocol && strings.Contains(str, "500 PROXY") {
						continue
					} else {
						lastError = fmt.Errorf("%s origin server has not support TLS connection", code)

						break
					}
				} else {
					// SSL/TLS wrapping on connection
					tlsConn := tls.Client(s.origin, s.tlsDatas.forOrigin.getTLSConfig())
					err = tlsConn.Handshake()
					if err != nil {
						return fmt.Errorf("TLS handshake with origin has failed")
					}

					s.log.debug("TLS control connection finished with origin. TLS protocol version: %s and Cipher Suite: %s", getTLSProtocolName(tlsConn.ConnectionState().Version), tls.CipherSuiteName(tlsConn.ConnectionState().CipherSuite))

					s.origin = tlsConn
					s.originReader = bufio.NewReader(s.origin)
					s.originWriter = bufio.NewWriter(s.origin)

					break
				}
			} else {
				break
			}
		}
	}

	return lastError
}

func (s *proxyServer) switchOrigin(clientAddr string, originAddr string, previousTLSCommands []string) error {
	// return error when user not found
	if len(originAddr) == 0 {
		return fmt.Errorf("user id not found")
	}

	// if client switched before, return error
	if s.isLoggedin {
		return fmt.Errorf("origin already switched")
	}

	s.log.info("switch origin to: %s", originAddr)
	var err error

	if s.passThrough {
		s.suspend()
		defer s.unsuspend()
	}

	// disconnect old origin and close response listener
	s.stopChan <- struct{}{}
	<-s.stopChanDone

	lastError := error(nil)
	switchResult := false

	defer func() {
		s.stop = false

		// send switching complate signal
		s.waitSwitching <- switchResult
	}()

	// change connection and reset reader and writer buffer
	s.origin, err = net.DialTimeout("tcp", originAddr, time.Duration(connectionTimeout)*time.Second)
	if err != nil {
		return err
	}
	s.originReader = bufio.NewReader(s.origin)
	s.originWriter = bufio.NewWriter(s.origin)

	// Send proxy protocol v1 header when set proxy protocol true
	if s.config.ProxyProtocol {
		s.log.debug("send proxy protocol to origin")
		if err := s.sendProxyHeader(clientAddr, originAddr); err != nil {
			return err
		}
	}

	// Read welcome message from ftp connection
	res, err := s.originReader.ReadString('\n')
	if err != nil {
		return errors.New("cannot connect to new origin server")
	}

	s.log.debug("response from new origin: %s", strings.TrimSuffix(res, "\r\n"))

	// set linger 0 and tcp keepalive setting between switched origin connection
	tcpConn := s.origin.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(s.config.KeepaliveTime) * time.Second)
	tcpConn.SetLinger(0)

	s.origin = tcpConn

	// If client connect with TLS connection, make TLS connection to origin ftp server too.
	if err := s.sendTLSCommand(previousTLSCommands); err != nil {
		return err
	}

	// set switch process complate
	switchResult = true

	return lastError
}

func (s *proxyServer) startProxy() error {
	// return if proxy still unsuspended or s.stop is true
	if s.stop || !s.passThrough {
		return nil
	}

	read := make(chan string)
	done := make(chan struct{})
	send := make(chan struct{})
	errchan := make(chan error)
	lastError := error(nil)

	go func() {
		for {
			s.isDataCommandResponse = false
			buff, err := s.originReader.ReadString('\n')
			if err != nil {
				if !s.stop {
					safeSetChanel(errchan, err)
				}
				break
			} else {
				if s.config.ProxyTimeout > 0 {
					// do not time out during transfer data
					if s.inDataTransfer.IsSet() {
						s.origin.SetDeadline(time.Time{})
					} else {
						s.origin.SetDeadline(time.Now().Add(time.Duration(s.config.ProxyTimeout) * time.Second))
					}
				}

				s.log.debug("response from origin: %s", strings.TrimSuffix(buff, "\r\n"))

				// response user setted welcome message
				if strings.Compare(getCode(buff)[0], "220") == 0 && !s.isLoggedin {
					buff = s.welcomeMsg
				}

				// check login and switch origin success
				if strings.Compare(getCode(buff)[0], "230") == 0 {
					s.isLoggedin = true
				}

				// when got 500 PROXY not understood, ignore it
				// this ignore setting for complex origins.
				// if some origins needs proxy protocol and some else is not,
				// pftp cannot support both in same time. So, pftp ignore the
				// 500 PROXY not understood then client can connect any servers.
				if s.config.ProxyProtocol && strings.Contains(buff, "500 PROXY") {
					continue
				}

				// is data channel proxy used
				if s.config.DataChanProxy && s.isLoggedin {
					if strings.HasPrefix(buff, "227 ") {
						s.isDataCommandResponse = true
						s.dataConnector.parsePASVresponse(buff)
					}
					if strings.HasPrefix(buff, "229 ") {
						s.isDataCommandResponse = true
						s.dataConnector.parseEPSVresponse(buff)
					}
					if strings.HasPrefix(buff, "200 PORT command successful") {
						s.isDataCommandResponse = true
					}

					// when got 150 from origin, it means data transfer has started
					// set transfer in progress flag to 1
					if strings.HasPrefix(buff, "150 ") {
						s.inDataTransfer.Set()
					}

					// when got 226 from origin, it means data transfer finished
					// set data transfer in p rogress flag to 0 for accept next data transfers
					if strings.HasPrefix(buff, "226 ") {
						s.inDataTransfer.UnSet()
					}

					if s.isDataCommandResponse {
						if s.isDataHandlerAvailable() {
							switch s.dataConnector.clientConn.mode {
							case "PORT", "EPRT":
								buff = fmt.Sprintf("200 %s command successful\r\n", s.dataConnector.clientConn.mode)
							case "PASV":
								// prepare PASV response line to client
								_, lPort, _ := net.SplitHostPort(s.dataConnector.clientConn.listener.Addr().String())
								listenPort, _ := strconv.Atoi(lPort)
								buff = fmt.Sprintf("227 Entering Passive Mode (%s,%s,%s).\r\n",
									strings.ReplaceAll(s.config.MasqueradeIP, ".", ","),
									strconv.Itoa(listenPort/256),
									strconv.Itoa(listenPort%256))
							case "EPSV":
								// prepare EPSV response line to client
								_, listenPort, _ := net.SplitHostPort(s.dataConnector.clientConn.listener.Addr().String())
								buff = fmt.Sprintf("229 Entering Extended Passive Mode (|||%s|).\r\n", listenPort)
							}
						} else {
							buff = "425 Can't open data connection\r\n"
						}
					}
				}

				// handling multi-line response
				if len(buff) >= 4 && buff[3] == '-' {
					params := getCode(buff)
					multiLine := buff

					for {
						res, err := s.originReader.ReadString('\n')
						if err != nil {
							safeSetChanel(errchan, err)
							done <- struct{}{}
							return
						}

						// store multi-line response
						multiLine += res

						// check multi-line end
						if getCode(res)[0] == params[0] && res[3] == ' ' {
							buff = multiLine
							break
						}
					}
				}

				if s.passThrough {
					read <- buff
					<-send
				}
			}
		}
		done <- struct{}{}
	}()

loop:
	for {
		select {
		case b := <-read:
			if err := s.sendToClient(strings.TrimRight(b, "\r\n")); err != nil {
				if !strings.Contains(err.Error(), alreadyClosedMsg) {
					s.log.err("error on write response to client: %s, err: %s", strings.TrimSuffix(b, "\r\n"), err.Error())
				}
			}
			send <- struct{}{}
		case err := <-errchan:
			lastError = err
			connectionCloser(s, s.log)

			break loop
		case <-s.stopChan:
			s.stop = true

			// close read goroutine
			connectionCloser(s, s.log)

			s.stopChanDone <- struct{}{}
			break loop
		}
	}
	<-done

	return lastError
}

// Hide parameters from log
func (s *proxyServer) commandLog(line string) {
	if strings.Compare(strings.ToUpper(getCommand(line)[0]), secureCommand) == 0 {
		s.log.debug("send to origin: %s ********", secureCommand)
	} else {
		s.log.debug("send to origin: %s", strings.TrimSuffix(line, "\r\n"))
	}
}

// split response line
func getCode(line string) []string {
	if len(line) >= 4 {
		return strings.SplitN(strings.Trim(line, "\r\n"), string(line[3]), 2)
	}

	return []string{line}
}
