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
	eg            errgroup.Group
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
		eg:         errgroup.Group{},
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
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			if server.listener != nil {
				return err
			}
		}

		if server.config.IdleTimeout > 0 {
			conn.SetDeadline(time.Now().Add(time.Duration(server.config.IdleTimeout) * time.Second))
		}

		server.clientCounter++
		c := newClientHandler(conn, server.config, server.middleware, server.clientCounter, &currentConnection)
		logrus.Info("FTP Client connected ", "clientIp ", conn.RemoteAddr())
		server.eg.Go(func() error {
			return c.handleCommands()
		})
	}
	return nil
}

func (server *FtpServer) wait() error {
	return server.eg.Wait()
}

func (server *FtpServer) ListenAndServe() error {
	if err := server.Listen(); err != nil {
		return err
	}

	logrus.Info("Starting...")

	return server.Serve()
}

func (server *FtpServer) Stop() error {
	if os.Getenv("SERVER_STARTER_PORT") == "" && server.listener != nil {
		return server.listener.Close()
	}
	return server.wait()
}
