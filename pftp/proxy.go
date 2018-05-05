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
	timeout int
	client  net.Conn
	origin  net.Conn
	doProxy bool
	pipe    chan []byte
}

func NewProxyServer(timeout int, client net.Conn, originAddr string) (*ProxyServer, error) {
	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return nil, err
	}

	p := &ProxyServer{
		client:  client,
		origin:  c,
		timeout: timeout,
		doProxy: true,
		pipe:    make(chan []byte, BUFFER_SIZE),
	}

	// read welcome message
	_, err = p.ReadFromOrigin()

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
			logrus.Debug("read from origin:", response)
			return response, nil
		}
	}
	return "", nil
}

func (s *ProxyServer) SendToOrigin(line string) error {
	logrus.Debug("send to origin:", line)
	if _, err := s.origin.Write([]byte(line)); err != nil {
		return err
	}

	return nil
}

// オリジンにコマンドを投げてから結果を受け取る
func (s *ProxyServer) SendAndReadFromOrigin(line string) (string, error) {
	err := s.SendToOrigin(line)
	if err != nil {
		return "", err
	}
	return s.ReadFromOrigin()
}

func (s *ProxyServer) SendToClient(line string) error {
	logrus.Debug("send to client:", line)
	if _, err := s.client.Write([]byte(line)); err != nil {
		return err
	}
	return nil

}

func (s *ProxyServer) Start(isUpload bool) error {
	defer s.Close()

	if isUpload {
		return s.start(s.client, s.origin)
	}
	return s.start(s.origin, s.client)
}

func (s *ProxyServer) Suspend() {
	logrus.Debug("suspend")
	s.doProxy = false
}

func (s *ProxyServer) Unsuspend() {
	s.doProxy = true
}

func (s *ProxyServer) Close() {
	s.client.Close()
	s.origin.Close()
}

func (s *ProxyServer) start(from, to net.Conn) error {
	logrus.Debug("relay start from=", from.LocalAddr(), " to=", from.RemoteAddr())
	defer to.Close()
	defer from.Close()

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
			read <- buff[:n]
		}
	}
	close(read)
	<-done

	return lastError
}
