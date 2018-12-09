package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

const (
	BUFFER_SIZE = 4096
)

type proxyServer struct {
	id            int
	timeout       int
	clientReader  *bufio.Reader
	clientWriter  *bufio.Writer
	originReader  *bufio.Reader
	originWriter  *bufio.Writer
	origin        net.Conn
	passThrough   bool
	mutex         *sync.Mutex
	log           *logger
	sem           int
	proxyProtocol bool
	stopChan      chan struct{}
	stop          bool
}

type proxyServerConfig struct {
	timeout       int
	clientReader  *bufio.Reader
	clientWriter  *bufio.Writer
	originAddr    string
	mutex         *sync.Mutex
	log           *logger
	proxyProtocol bool
}

func newProxyServer(conf *proxyServerConfig) (*proxyServer, error) {
	c, err := net.Dial("tcp", conf.originAddr)
	if err != nil {
		return nil, err
	}

	p := &proxyServer{
		clientReader:  conf.clientReader,
		clientWriter:  conf.clientWriter,
		originWriter:  bufio.NewWriter(c),
		originReader:  bufio.NewReader(c),
		origin:        c,
		timeout:       conf.timeout,
		passThrough:   true,
		mutex:         conf.mutex,
		log:           conf.log,
		proxyProtocol: conf.proxyProtocol,
		stopChan:      make(chan struct{}),
	}
	p.log.debug("new proxy from=%s to=%s", c.LocalAddr(), c.RemoteAddr())

	return p, err
}

func (s *proxyServer) sendToOrigin(line string) error {
	cnt := 0
	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}

	for {
		if cnt > s.timeout {
			return errors.New("Could not get semaphore to send to client")
		}

		s.log.debug("send to origin: %s", line)

		if s.semFree() {
			if _, err := s.origin.Write([]byte(line)); err != nil {
				return err
			}
			s.semLock()
			break
		}
		time.Sleep(1 * time.Second)
		cnt++
	}
	return nil
}

func (s *proxyServer) responseProxy() error {
	return s.start(s.originReader, s.clientWriter)
}

func (s *proxyServer) semLock() {
	s.sem++
}

func (s *proxyServer) semUnlock() {
	s.sem--
}

func (s *proxyServer) semFree() bool {
	return s.sem < 1
}

func (s *proxyServer) semLocked() bool {
	return s.sem > 0
}

func (s *proxyServer) suspend() error {
	s.log.debug("suspend proxy")
	cnt := 0
	for {
		if cnt > s.timeout {
			return errors.New("Could not get semaphore to send to client")
		}

		if s.semFree() {
			s.passThrough = false
			break
		}
		time.Sleep(1 * time.Second)
		cnt++
	}
	return nil
}

func (s *proxyServer) unsuspend() {
	s.log.debug("unsuspend proxy")
	s.passThrough = true
}

func (s *proxyServer) Close() {
	s.origin.Close()
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

/* send TLS command to origin.                   *
*  TLS version set to TLSv1 forcebly because     *
*  client/pftp/origin must set same TLS version. */
func (s *proxyServer) sendTLSCommand() error {
	config := tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
		MaxVersion:         tls.VersionTLS10,
	}

	// send AUTH command
	if _, err := s.origin.Write([]byte("AUTH TLS\r\n")); err != nil {
		return err
	}
	// read response
	if _, err := s.originReader.ReadString('\n'); err != nil {
		return err
	}

	// TLS wrapping on connection
	s.origin = tls.Client(s.origin, &config)
	s.originReader = bufio.NewReader(s.origin)
	s.originWriter = bufio.NewWriter(s.origin)

	return nil
}

func (s *proxyServer) switchOrigin(clientAddr string, originAddr string, useTLS bool, previousTLSCommands []string) error {
	s.log.debug("switch origin to: %s", originAddr)

	if s.passThrough {
		err := s.suspend()
		if err != nil {
			return err
		}
		defer s.unsuspend()
	}

	s.stopChan <- struct{}{}

	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return err
	}

	old := s.origin
	s.origin = c

	// Send proxy protocol v1 header when set proxy protocol true
	if s.proxyProtocol {
		err := s.sendProxyHeader(clientAddr, originAddr)
		if err != nil {
			return err
		}
	}

	reader := bufio.NewReader(c)
	writer := bufio.NewWriter(c)
	// read welcome message
	if _, err := reader.ReadString('\n'); err != nil {
		return err
	}

	*s.originReader = *reader
	*s.originWriter = *writer
	old.Close()

	// If client connect with TLS connection, make TLS connection to origin ftp server too.
	if useTLS {
		err := s.sendTLSCommand()
		if err != nil {
			return err
		}
	}

	/* If client send PBSZ and PROT commands before login,  *
	*  send stored command line to origin for set same PBSZ *
	*  and PROT options                                     */
	for i := 0; i < len(previousTLSCommands); i++ {
		s.log.debug("send to origin: %s", previousTLSCommands[i])
		if _, err := s.origin.Write([]byte(previousTLSCommands[i])); err != nil {
			return err
		}

		// read response
		if _, err := reader.ReadString('\n'); err != nil {
			return err
		}
	}

	s.stop = false
	return nil
}

func (s *proxyServer) start(from *bufio.Reader, to *bufio.Writer) error {
	if s.stop {
		return nil
	}

	buff := make([]byte, BUFFER_SIZE)
	read := make(chan []byte, BUFFER_SIZE)
	done := make(chan struct{})
	send := make(chan struct{})
	errchan := make(chan error)
	var lastError error

	go func() {
		for {
			if n, err := from.Read(buff); err != nil {
				if err != io.EOF {
					safeSetChanel(errchan, err)
				} else {
					// when receive EOS from origin, send EOF to client for terminate session completely
					if s.passThrough || s.semLocked() {
						read <- buff[:n]
						if s.semLocked() {
							s.semUnlock()
						}
					}
					<-send

					lastError = err
					s.stopChan <- struct{}{}
				}
				break
			} else {
				if s.timeout > 0 {
					s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
				}

				if s.passThrough || s.semLocked() {
					read <- buff[:n]
					if s.semLocked() {
						s.semUnlock()
					}
				}
			}
			<-send
		}
		done <- struct{}{}
	}()

loop:
	for {
		select {
		case b, ok := <-read:
			if !ok {
				break loop
			}

			s.mutex.Lock()
			_, err := to.Write(b)
			if err != nil {
				lastError = err
				s.mutex.Unlock()
				break loop
			}

			if err := to.Flush(); err != nil {
				lastError = err
				s.mutex.Unlock()
				break loop
			}
			s.mutex.Unlock()
			send <- struct{}{}
		case err := <-errchan:
			lastError = err
			break loop
		case <-s.stopChan:
			close(errchan)
			// close read groutine
			s.origin.Close()
			s.stop = true
			break loop
		}
	}
	close(read)
	<-done

	return lastError
}
