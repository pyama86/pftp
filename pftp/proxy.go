package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

const (
	BUFFER_SIZE    = 4096
	SECURE_COMMAND = "PASS"
)

type proxyServer struct {
	id             int
	timeout        int
	clientReader   *bufio.Reader
	clientWriter   *bufio.Writer
	originReader   *bufio.Reader
	originWriter   *bufio.Writer
	origin         net.Conn
	passThrough    bool
	mutex          *sync.Mutex
	log            *logger
	proxyProtocol  bool
	stopChan       chan struct{}
	stopChanDone   chan struct{}
	established    chan struct{}
	stop           bool
	secureCommands []string
	isSwitched     bool
	welcomeMsg     string
}

type proxyServerConfig struct {
	timeout        int
	clientReader   *bufio.Reader
	clientWriter   *bufio.Writer
	originAddr     string
	mutex          *sync.Mutex
	log            *logger
	proxyProtocol  bool
	welcomeMsg     string
	secureCommands []string
	established    chan struct{}
}

func newProxyServer(conf *proxyServerConfig) (*proxyServer, error) {
	c, err := net.Dial("tcp", conf.originAddr)
	if err != nil {
		return nil, err
	}

	// set conn to TCPConn
	tcpConn := c.(*net.TCPConn)

	// set linger 0 to local origin ftp server
	tcpConn.SetLinger(0)

	p := &proxyServer{
		clientReader:   conf.clientReader,
		clientWriter:   conf.clientWriter,
		originWriter:   bufio.NewWriter(c),
		originReader:   bufio.NewReader(c),
		origin:         tcpConn,
		timeout:        conf.timeout,
		passThrough:    true,
		mutex:          conf.mutex,
		log:            conf.log,
		proxyProtocol:  conf.proxyProtocol,
		stopChan:       make(chan struct{}),
		stopChanDone:   make(chan struct{}),
		welcomeMsg:     "220 " + conf.welcomeMsg + "\r\n",
		secureCommands: conf.secureCommands,
		isSwitched:     false,
		established:    conf.established,
	}
	p.log.debug("new proxy from=%s to=%s", c.LocalAddr(), c.RemoteAddr())

	return p, err
}

func (s *proxyServer) sendToOrigin(line string) error {
	// command line must contain CRLF only once in the end
	if !strings.HasSuffix(line, "\r\n") || strings.Count(line, "\r") != 1 || strings.Count(line, "\n") != 1 {
		s.log.debug("wrong command line. make line end by CRLF")

		// delete CR & LF characters from line (only allow to end of line "\r\n")
		line = strings.Replace(line, "\n", "", -1)
		line = strings.Replace(line, "\r", "", -1)

		// add write CRLF to end of line
		line += "\r\n"
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

func (s *proxyServer) responseProxy() error {
	return s.start(s.originReader, s.clientWriter)
}

func (s *proxyServer) suspend() {
	s.log.debug("suspend proxy")
	s.passThrough = false
}

func (s *proxyServer) unsuspend() {
	s.log.debug("unsuspend proxy")
	s.passThrough = true
}

func (s *proxyServer) Close() error {
	err := s.origin.Close()
	return err
}

func (s *proxyServer) sendProxyHeader(clientAddr string, originAddr string) error {
	sourceAddr := strings.Split(clientAddr, ":")
	destinationAddr := strings.Split(originAddr, ":")
	sourcePort, _ := strconv.Atoi(sourceAddr[1])
	destinationPort, _ := strconv.Atoi(destinationAddr[1])

	// proxyProtocolHeader's DestinationAddress must be IP! not domain name
	hostIP, err := net.LookupIP(destinationAddr[0])
	if err != err {
		return err
	}

	proxyProtocolHeader := proxyproto.Header{
		Version:            byte(1),
		Command:            proxyproto.PROXY,
		TransportProtocol:  proxyproto.TCPv4,
		SourceAddress:      net.ParseIP(sourceAddr[0]),
		DestinationAddress: net.ParseIP(hostIP[0].String()),
		SourcePort:         uint16(sourcePort),
		DestinationPort:    uint16(destinationPort),
	}

	_, err = proxyProtocolHeader.WriteTo(s.origin)
	return err
}

/* send command before login to origin.                  *
*  TLS version set by client to pftp tls version         *
*  because client/pftp/origin must set same TLS version. */
func (s *proxyServer) sendTLSCommand(tlsProtocol uint16, previousTLSCommands []string) error {
	lastError := error(nil)

	for _, cmd := range previousTLSCommands {
		s.commandLog(cmd)
		if _, err := s.originWriter.WriteString(cmd); err != nil {
			return fmt.Errorf("failed to send AUTH command to origin")
		}
		if err := s.originWriter.Flush(); err != nil {
			return err
		}

		// Read response from new origin server
		str, err := s.originReader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to make TLS connection")
		}

		if strings.Compare(strings.ToUpper(getCommand(cmd)[0]), "AUTH") == 0 {
			code := getCode(str)[0]
			if code != "234" {
				lastError = fmt.Errorf("%s origin server has not support TLS connection", code)
				break
			} else {
				config := tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tlsProtocol,
					MaxVersion:         tlsProtocol,
				}

				s.log.debug("set TLS connection")
				// SSL/TLS wrapping on connection
				s.origin = tls.Client(s.origin, &config)
				s.originReader = bufio.NewReader(s.origin)
				s.originWriter = bufio.NewWriter(s.origin)
			}
		}
	}

	return lastError
}

func (s *proxyServer) switchOrigin(clientAddr string, originAddr string, tlsProtocol uint16, previousTLSCommands []string) error {
	s.log.info("switch origin to: %s", originAddr)
	var err error

	s.isSwitched = true

	if s.passThrough {
		s.suspend()
		defer s.unsuspend()
	}

	// disconnect old origin and close response listener
	s.stopChan <- struct{}{}
	<-s.stopChanDone

	cnt := 0
	lastError := error(nil)

	// if connection to new origin close immediatly, reconnect while proxy timeout
	for {
		// change connection and reset reader and writer buffer
		if s.origin, err = net.Dial("tcp", originAddr); err != nil {
			return err
		}
		s.originReader = bufio.NewReader(s.origin)
		s.originWriter = bufio.NewWriter(s.origin)

		// Send proxy protocol v1 header when set proxy protocol true
		if s.proxyProtocol {
			if err := s.sendProxyHeader(clientAddr, originAddr); err != nil {
				return err
			}
		}

		// Read welcome message from ftp connection
		res, err := s.originReader.ReadString('\n')
		if err != nil {
			if cnt > s.timeout {
				return errors.New("cannot connect to new origin server")
			}

			s.log.err("err from new origin: %s", err.Error())
			s.log.debug("reconnect to origin")
			cnt++

			s.Close()

			// reconnect interval
			time.Sleep(1 * time.Second)
			continue
		} else {
			s.log.debug("response from new origin: %s", res)
			break
		}
	}

	// If client connect with TLS connection, make TLS connection to origin ftp server too.
	if err := s.sendTLSCommand(tlsProtocol, previousTLSCommands); err != nil {
		lastError = err
	}

	s.stop = false
	return lastError
}

func (s *proxyServer) start(from *bufio.Reader, to *bufio.Writer) error {
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
			buff, err := from.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					s.log.debug("got EOF from origin")
				}

				if !s.stop {
					safeSetChanel(errchan, err)
				}
				break
			} else {
				if s.timeout > 0 {
					s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
				}

				// response user setted welcome message
				if strings.Compare(getCode(buff)[0], "220") == 0 && !s.isSwitched {
					buff = s.welcomeMsg

					// send first response complate signal
					if s.established != nil {
						s.established <- struct{}{}
					}
				}

				if buff[3] == '-' {
					params := getCode(buff)
					multiLine := buff

					for {
						res, err := from.ReadString('\n')
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

				s.log.debug("response from origin: %s", buff)

				if s.passThrough {
					read <- buff
				}

				<-send
			}
		}
		done <- struct{}{}
	}()

loop:
	for {
		select {
		case b := <-read:
			s.mutex.Lock()
			if _, err := to.WriteString(b); err != nil {
				s.mutex.Unlock()
				lastError = err
				s.Close()
				send <- struct{}{}

				break loop
			}

			if err := to.Flush(); err != nil {
				s.mutex.Unlock()
				lastError = err
				s.Close()
				send <- struct{}{}

				break loop
			}
			s.mutex.Unlock()
			send <- struct{}{}
		case err := <-errchan:
			lastError = err
			s.Close()

			break loop
		case <-s.stopChan:
			s.stop = true

			// close read groutine
			s.Close()

			s.stopChanDone <- struct{}{}
			break loop
		}
	}
	<-done

	return lastError
}

// Hide parameters from log
func (s *proxyServer) commandLog(line string) {
	if strings.Compare(strings.ToUpper(getCommand(line)[0]), SECURE_COMMAND) == 0 {
		s.log.debug("send to origin: %s ********\r\n", SECURE_COMMAND)
	} else {
		s.log.debug("send to origin: %s", line)
	}
}

// response line
func getCode(line string) []string {
	return strings.SplitN(strings.Trim(line, "\r\n"), string(line[3]), 2)
}
