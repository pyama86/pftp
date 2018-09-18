package pftp

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/lestrrat/go-server-starter/listener"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type middlewareFunc func(*Context, string) error
type middleware map[string]middlewareFunc

type FtpServer struct {
	listener      net.Listener
	clientCounter int
	config        *config
	middleware    middleware
	shutdown      bool
}

func NewFtpServer(confFile string) (*FtpServer, error) {
	c, err := loadConfig(confFile)
	if err != nil {
		return nil, err
	}
	m := middleware{}
	return &FtpServer{
		config:     c,
		middleware: m,
	}, nil
}

func (server *FtpServer) Use(command string, m middlewareFunc) {
	server.middleware[strings.ToUpper(command)] = m
}

func (server *FtpServer) Listen() (err error) {
	if os.Getenv("SERVER_STARTER_PORT") != "" {
		listeners, err := listener.ListenAll()
		if listeners == nil || err != nil {
			return err
		}
		server.listener = listeners[0]
	} else {
		l, err := net.Listen("tcp", server.config.ListenAddr)
		if err != nil {
			return err
		}
		server.listener = l
	}

	logrus.Info("Listening address ", server.listener.Addr())

	return err
}

func (server *FtpServer) Serve() error {
	var currentConnection int32
	currentConnection = 0
	eg := errgroup.Group{}
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			if server.listener != nil {
				return err
			}
		}

		if server.shutdown && strings.HasPrefix(conn.RemoteAddr().String(), "127.0.0.1") {
			break
		}

		if server.config.IdleTimeout > 0 {
			conn.SetDeadline(time.Now().Add(time.Duration(server.config.IdleTimeout) * time.Second))
		}

		server.clientCounter++
		c := newClientHandler(conn, server.config, server.middleware, server.clientCounter, &currentConnection)
		logrus.Info("FTP Client connected ", "clientIp ", conn.RemoteAddr())
		eg.Go(func() error {
			return c.handleCommands()
		})
	}
	return eg.Wait()
}

func (server *FtpServer) ListenAndServe() error {
	if err := server.Listen(); err != nil {
		return err
	}

	logrus.Info("Starting...")

	return server.Serve()
}

func (server *FtpServer) Stop() error {
	if os.Getenv("SERVER_STARTER_PORT") != "" {
		server.shutdown = true
		listeners, err := listener.Ports()
		if err != nil {
			return err
		}

		pair := strings.Split(listeners[0].String(), "=")
		// send close message
		conn, err := net.Dial("tcp", pair[0])
		if err != nil {
			return err
		}
		defer conn.Close()
	} else if server.listener != nil {
		return server.listener.Close()
	}
	return nil
}
