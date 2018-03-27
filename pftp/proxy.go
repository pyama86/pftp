package pftp

import (
	"bufio"
	"fmt"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	BUFFER_SIZE = 0xFFFF
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

	logrus.Debug("client:", client.LocalAddr(), " origin:", c.RemoteAddr())
	return &ProxyServer{
		client:       client,
		origin:       c,
		originReader: bufio.NewReader(c),
		timeout:      timeout,
	}, nil
}

func (s *ProxyServer) ReadLine() (string, error) {

	if s.timeout > 0 {
		s.origin.SetReadDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(s.timeout))))
	}
	for {
		if response, err := s.originReader.ReadString('\n'); err != nil {
			return "", err
		} else {
			logrus.Debug("response command:", response)
			return response, nil
		}
	}
	return "", nil
}

func (s *ProxyServer) SendLine(line string) error {
	logrus.Debug("send command:", line)
	if _, err := s.origin.Write([]byte(line)); err != nil {
		return err
	}

	return nil
}

func (s *ProxyServer) SendWithReadLine(line string) (string, error) {
	err := s.SendLine(line)
	if err != nil {
		return "", err
	}
	return s.ReadLine()
}

func (s *ProxyServer) SendLineWithProxy(line string) error {
	err := s.SendLine(line)
	if err != nil {
		return err
	}

	response, err := s.ReadLine()
	if err != nil {
		return err
	}

	return s.SendClient(response)
}

func (s *ProxyServer) SendClient(line string) error {
	if _, err := s.client.Write([]byte(line)); err != nil {
		return err
	}
	return nil

}

func (s *ProxyServer) writeCmd(conn net.Conn, cmd, value string) error {
	if _, err := conn.Write([]byte(fmt.Sprintf("%s %s", cmd, value))); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("\r\n")); err != nil {
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

func (s *Proxy) writeCmd(conn net.Conn, cmd, value string) error {
	if _, err := conn.Write([]byte(fmt.Sprintf("%s %s", cmd, value))); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("\r\n")); err != nil {
		return err
	}
	return nil
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
	logrus.Debugf("from=", fromConn.LocalAddr())
	logrus.Debugf("to=", fromConn.RemoteAddr())
	buff := make([]byte, BUFFER_SIZE)
	for {
		n, err := fromConn.Read(buff)
		if err != nil {
			return err
		}
		b := buff[:n]
		logrus.Info(string(b))
		n, err = toConn.Write(b)
		if err != nil {
			return err
		}
	}
}
