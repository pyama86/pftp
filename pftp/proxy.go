package pftp

import (
	"bufio"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	BUFFER_SIZE = 0xFFFF
)

type ProxyServer struct {
	user   string
	local  net.Conn
	remote net.Conn
}

func NewProxyServer(user string, local net.Conn, remoteAddr string) (*ProxyServer, error) {
	c, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		return nil, err
	}

	remote := bufio.NewReader(c)
	// read welcome message
	if _, err := remote.ReadString('\n'); err != nil {
		return nil, err
	}

	return &ProxyServer{
		user:   user,
		local:  local,
		remote: c,
	}, nil
}

func (s *ProxyServer) SendLine(line string) error {
	rremote := bufio.NewReader(s.remote)
	if _, err := s.remote.Write([]byte(line)); err != nil {
		return err
	}

	for {
		if response, err := rremote.ReadString('\n'); err != nil {
			return err
		} else {
			if _, err := s.local.Write([]byte(response)); err != nil {
				return err
			} else {
				return nil
			}
		}
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
	defer s.local.Close()
	defer s.remote.Close()

	p := &Proxy{}
	return p.Start(s.user, s.local, s.remote)
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

func (p *Proxy) Start(username string, clientConn, serverConn net.Conn) error {
	defer clientConn.Close()
	defer serverConn.Close()

	var eg errgroup.Group

	eg.Go(func() error { return p.relay(&eg, serverConn, clientConn) })
	eg.Go(func() error { return p.relay(&eg, clientConn, serverConn) })
	if err := p.writeCmd(serverConn, "USER", username); err != nil {
		return nil
	}

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
		logrus.Info("remote=", fromConn.RemoteAddr())
		logrus.Info("local=", toConn.RemoteAddr())
		logrus.Info(string(b))
		n, err = toConn.Write(b)
		if err != nil {
			return err
		}
	}
}
