package pftp

import (
	"bufio"
	"context"
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

func (s *ProxyServer) Start() error {
	defer s.Close()
	p := &Proxy{}
	return p.Start(s.client, s.origin)
}

func (s *ProxyServer) Close() {
	s.client.Close()
	s.origin.Close()
}

type Proxy struct{}

func (p *Proxy) Start(clientConn, serverConn net.Conn) error {
	ctx, done := context.WithCancel(context.Background())
	defer done()
	eg, ctx := errgroup.WithContext(ctx)

	// リレー用のgoroutineを起動
	eg.Go(func() error { return p.relay(ctx, serverConn, clientConn) })
	eg.Go(func() error { return p.relay(ctx, clientConn, serverConn) })

	// 完了まで待ち合わせる
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}

func (p *Proxy) relay(ctx context.Context, fromConn, toConn net.Conn) error {
	logrus.Debug("relay start from=", fromConn.LocalAddr(), " to=", fromConn.RemoteAddr())
	buff := make([]byte, BUFFER_SIZE)
	errChan := make(chan error, 1)
	read := make(chan []byte, BUFFER_SIZE)

	// ソケットからの読込をgoroutineにすることで非同期IO
	go func() {
		for {
			n, err := fromConn.Read(buff)
			if err != nil {
				errChan <- err
				return
			}
			read <- buff[:n]
		}
	}()

	for {
		select {
		case <-ctx.Done():
			// todo connection close
			return ctx.Err()
		case err := <-errChan:
			return err
		case b := <-read:
			_, err := toConn.Write(b)
			if err != nil {
				return err
			}
		}
	}
}
