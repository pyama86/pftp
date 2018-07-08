package pftp

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"time"

	"github.com/sirupsen/logrus"
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
	pipe         chan []byte
	CloseOk      bool
	Switch       bool
}

func NewProxyServer(timeout int, clientReader *bufio.Reader, clientWriter *bufio.Writer, originAddr string, id int) (*ProxyServer, error) {
	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("[%d]new proxy from=%s to=%s", id, c.LocalAddr(), c.RemoteAddr())
	p := &ProxyServer{
		id:           id,
		clientReader: clientReader,
		clientWriter: clientWriter,
		originWriter: bufio.NewWriter(c),
		originReader: bufio.NewReader(c),
		origin:       c,
		timeout:      timeout,
		doProxy:      true,
		pipe:         make(chan []byte, BUFFER_SIZE),
	}
	p.CloseOk = false
	p.Switch = false

	return p, err
}

func (s *ProxyServer) ReadFromOrigin() (string, error) {
	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}

	var reader *bufio.Reader
	if s.doProxy {
		reader = bufio.NewReader(s.origin)
	} else {
		reader = bufio.NewReader(bytes.NewBuffer(<-s.pipe))
	}

	for {
		if response, err := reader.ReadString('\n'); err != nil {
			return "", err
		} else {
			logrus.Debugf("[%d]read from origin:%s", s.id, response)
			return response, nil
		}
	}
	return "", nil
}

func (s *ProxyServer) SendToOrigin(line string) error {
	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}

	logrus.Debugf("[%d]send to origin:%s", s.id, line)
	if _, err := s.origin.Write([]byte(line)); err != nil {
		return err
	}

	return nil
}

func (s *ProxyServer) SendToClient(line string) error {
	logrus.Debugf("[%d]send to client:%s", s.id, line)
	if _, err := s.clientWriter.Write([]byte(line)); err != nil {
		return err
	}

	if err := s.clientWriter.Flush(); err != nil {
		return err
	}

	return nil

}

func (s *ProxyServer) UploadProxy() error {
	return s.start(s.clientReader, s.originWriter)
}

func (s *ProxyServer) DownloadProxy() error {
	return s.start(s.originReader, s.clientWriter)
}

func (s *ProxyServer) Suspend() {
	logrus.Debugf("[%d]suspend proxy", s.id)
	s.doProxy = false
}

func (s *ProxyServer) Unsuspend() {
	logrus.Debugf("[%d]unsuspend proxy", s.id)
	s.doProxy = true
}

func (s *ProxyServer) Close() {
	s.CloseOk = true
	s.origin.Close()
}

func (s *ProxyServer) SwitchOrigin(originAddr string) error {
	logrus.Debugf("[%d]switch origin to: %s", s.id, originAddr)

	if s.doProxy {
		s.Suspend()
		defer s.Unsuspend()
	}

	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return err
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

				if s.doProxy {
					_, err := to.Write(b)
					if err != nil {
						lastError = err
						break loop
					}

					if err := to.Flush(); err != nil {
						lastError = err
						break loop
					}
				} else {
					s.pipe <- b
				}
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
			read <- buff[:n]
		}
	}
	close(read)
	<-done

	return lastError
}
