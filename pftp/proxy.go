package pftp

import (
	"fmt"
	"net"

	"golang.org/x/sync/errgroup"
)

const (
	BUFFER_SIZE = 0xFFFF
)

type ProxyServer struct {
	local  net.Conn
	remote net.Conn
}

func NewProxyServer(local net.Conn, remoteAddr string) (*ProxyServer, error) {
	c, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		return nil, err
	}

	return &ProxyServer{
		local:  local,
		remote: c,
	}, nil
}

func (s *ProxyServer) Start() error {
	for {
		go s.handleConn()
	}
}

func (s *ProxyServer) WriteRemote(cmd, value string) error {
	if _, err := s.remote.Write([]byte(fmt.Sprintf("%s %s", cmd, value))); err != nil {
		return err
	}
	if _, err := s.remote.Write([]byte("\r\n")); err != nil {

		return err
	}
	return nil
}

func (s *ProxyServer) handleConn() error {
	defer s.local.Close()
	defer s.remote.Close()

	var eg errgroup.Group

	eg.Go(func() error { return s.relay(&eg, s.local, s.remote) })
	eg.Go(func() error { return s.relay(&eg, s.remote, s.local) })

	return eg.Wait()
}

func (p *ProxyServer) relay(eg *errgroup.Group, fromConn, toConn net.Conn) error {
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
