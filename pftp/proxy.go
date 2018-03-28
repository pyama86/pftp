package pftp

import (
	"bufio"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	BUFFER_SIZE = 0xFFFF
)

type ProxyServer struct {
	timeout int
	client  net.Conn
	origin  net.Conn
}

func NewProxyServer(timeout int, client net.Conn, originAddr string) (*ProxyServer, error) {
	c, err := net.Dial("tcp", originAddr)
	if err != nil {
		return nil, err
	}

	logrus.Debug("client:", client.LocalAddr(), " origin:", c.RemoteAddr())
	return &ProxyServer{
		client:  client,
		origin:  c,
		timeout: timeout,
	}, nil
}

func (s *ProxyServer) ReadFromOrigin() (string, error) {
	originReader := bufio.NewReader(s.origin)
	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}

	for {
		if response, err := originReader.ReadString('\n'); err != nil {
			return "", err
		} else {
			logrus.Debug("response command:", response)
			return response, nil
		}
	}
	return "", nil
}

func (s *ProxyServer) SendToOrigin(line string) error {
	logrus.Debug("send command:", line)
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

// オリジンにコマンドを投げてから結果をクライアントにプロキシする
func (s *ProxyServer) SendToOriginWithProxy(line string) error {
	err := s.SendToOrigin(line)
	if err != nil {
		return err
	}

	response, err := s.ReadFromOrigin()
	if err != nil {
		return err
	}

	return s.SendToClient(response)
}

func (s *ProxyServer) SendToClient(line string) error {
	if _, err := s.client.Write([]byte(line)); err != nil {
		return err
	}
	return nil

}

func (s *ProxyServer) Start() error {
	defer s.client.Close()
	defer s.origin.Close()

	p := &Proxy{}
	return p.Start(s.client, s.origin)
}

type Proxy struct{}

func (p *Proxy) Start(clientConn, serverConn net.Conn) error {
	defer clientConn.Close()
	defer serverConn.Close()

	var eg errgroup.Group

	eg.Go(func() error { return p.relay(&eg, serverConn, clientConn) })
	eg.Go(func() error { return p.relay(&eg, clientConn, serverConn) })

	return eg.Wait()
}

func (p *Proxy) relay(eg *errgroup.Group, fromConn, toConn net.Conn) error {
	buff := make([]byte, BUFFER_SIZE)
	for {
		n, err := fromConn.Read(buff)
		if err != nil {
			return err
		}
		b := buff[:n]
		n, err = toConn.Write(b)
		if err != nil {
			return err
		}
	}
}
