package pftp

import (
	"bufio"
	"io"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	BUFFER_SIZE = 4096
)

type ProxyServer struct {
	timeout      int
	client       net.Conn
	origin       net.Conn
	originReader *bufio.Reader
}

func NewProxyServer(timeout int, client net.Conn, originAddr string) (*ProxyServer, error) {
	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return nil, err
	}

	return &ProxyServer{
		client:       client,
		origin:       c,
		originReader: bufio.NewReader(c),
		timeout:      timeout,
	}, nil
}

func (s *ProxyServer) ReadFromOrigin() (string, error) {
	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}

	for {
		if response, err := s.originReader.ReadString('\n'); err != nil {
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
func (s *ProxyServer) SendAndReadToOrigin(line string) (string, error) {
	err := s.SendToOrigin(line)
	if err != nil {
		return "", err
	}
	return s.ReadFromOrigin()
}
func (s *ProxyServer) ReadFromOriginWithProxy() error {
	line, err := s.ReadFromOrigin()
	if err != nil {
		return err
	}

	return s.SendToClient(line)
}

// オリジンにコマンドを投げてから結果をクライアントにプロキシする
func (s *ProxyServer) SendToOriginWithProxy(line string) error {
	response, err := s.SendAndReadToOrigin(line)
	if err != nil {
		return err
	}
	return s.SendToClient(response)
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
	p := &Proxy{}

	if isUpload {
		return p.Start(s.client, s.origin)
	}
	return p.Start(s.origin, s.client)
}

func (s *ProxyServer) Close() {
	s.client.Close()
	s.origin.Close()
}

type Proxy struct{}

func (p *Proxy) Start(from, to net.Conn) error {
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

				_, err := to.Write(b)
				if err != nil {
					lastError = err
					break loop
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
