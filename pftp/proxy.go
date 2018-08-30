package pftp

import (
	"bufio"
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

type ProxyServer struct {
	id           int
	timeout      int
	clientReader *bufio.Reader
	clientWriter *bufio.Writer
	originReader *bufio.Reader
	originWriter *bufio.Writer
	origin       net.Conn
	doProxy      bool
	CloseOk      bool
	Switch       bool
	mutex        *sync.Mutex
	log          *logger
	sem          int
}

func NewProxyServer(timeout int, clientReader *bufio.Reader, clientWriter *bufio.Writer, originAddr string, m *sync.Mutex, l *logger) (*ProxyServer, error) {
	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return nil, err
	}

	p := &ProxyServer{
		clientReader: clientReader,
		clientWriter: clientWriter,
		originWriter: bufio.NewWriter(c),
		originReader: bufio.NewReader(c),
		origin:       c,
		timeout:      timeout,
		doProxy:      true,
		mutex:        m,
		log:          l,
	}
	p.CloseOk = false
	p.Switch = false
	l.debug("new proxy from=%s to=%s", c.LocalAddr(), c.RemoteAddr())

	return p, err
}

func (s *ProxyServer) SendToOrigin(line string) error {
	cnt := 0
	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}

	for {
		if cnt > s.timeout {
			return errors.New("Could not get semaphore to send to client")
		}

		s.log.debug("send to origin:%s", line)

		if s.sem < 1 {
			if _, err := s.origin.Write([]byte(line)); err != nil {
				return err
			}
			s.sem++
			break
		}
		time.Sleep(1 * time.Second)
		cnt++
	}
	return nil
}

func (s *ProxyServer) UploadProxy() error {
	return s.start(s.clientReader, s.originWriter)
}

func (s *ProxyServer) DownloadProxy() error {
	return s.start(s.originReader, s.clientWriter)
}

func (s *ProxyServer) Suspend() error {
	s.log.debug("suspend proxy")
	cnt := 0
	for {
		if cnt > s.timeout {
			return errors.New("Could not get semaphore to send to client")
		}

		if s.sem < 1 {
			s.doProxy = false
			break
		}
		time.Sleep(1 * time.Second)
		cnt++
	}
	return nil
}

func (s *ProxyServer) Unsuspend() {
	s.log.debug("unsuspend proxy")
	s.doProxy = true
}

func (s *ProxyServer) Close() {
	s.CloseOk = true
	s.origin.Close()
}

func (s *ProxyServer) SwitchOrigin(clientAddr string, originAddr string, proxyProtocol bool) error {
	s.log.debug("switch origin to: %s", originAddr)

	if s.doProxy {
		err := s.Suspend()
		if err != nil {
			return err
		}
		defer s.Unsuspend()
	}

	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return err
	}

	// Send proxy protocol v1 header when set proxy protocol true
	if proxyProtocol {
		sourceAddr := strings.Split(clientAddr, ":")
		destinationAddr := strings.Split(originAddr, ":")
		sourcePort, _ := strconv.Atoi(sourceAddr[1])
		destinationPort, _ := strconv.Atoi(destinationAddr[1])

		proxyProtocolHeader := proxyproto.Header{
			Version:            byte(1),
			Command:            proxyproto.PROXY,
			TransportProtocol:  proxyproto.TCPv4,
			SourceAddress:      net.ParseIP(sourceAddr[0]),
			DestinationAddress: net.ParseIP(destinationAddr[0]),
			SourcePort:         uint16(sourcePort),
			DestinationPort:    uint16(destinationPort),
		}

		proxyProtocolHeader.WriteTo(c)
	}

	old := s.origin
	s.origin = c

	reader := bufio.NewReader(c)
	writer := bufio.NewWriter(c)
	// read welcome message
	if _, err := reader.ReadString('\n'); err != nil {
		return err
	}

	*s.originReader = *reader
	*s.originWriter = *writer

	s.Switch = true
	old.Close()

	return nil
}

func (s *ProxyServer) start(from *bufio.Reader, to *bufio.Writer) error {

	buff := make([]byte, BUFFER_SIZE)
	read := make(chan []byte, BUFFER_SIZE)
	done := make(chan struct{})
	var lastError error

	go func() {
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
			}
		}
		done <- struct{}{}
	}()

	for {
		if n, err := from.Read(buff); err != nil {
			if err != io.EOF {
				lastError = err
			}
			break
		} else {
			if s.timeout > 0 {
				s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
			}

			if s.doProxy || s.sem > 0 {
				read <- buff[:n]
			}

			if s.sem > 0 {
				s.sem--
			}
		}
	}
	close(read)
	<-done

	return lastError
}
