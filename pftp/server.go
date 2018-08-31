package pftp

import (
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type middlewareFunc func(*Context, string) error
type middleware map[string]middlewareFunc

type FtpServer struct {
	listener      net.Listener
	clientCounter int
	config        *config
	middleware    middleware
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
	server.listener, err = net.Listen("tcp", server.config.ListenAddr)

	if err != nil {
		return err
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
		go func() {
			err := c.handleCommands()
			if err != nil {
				c.log.err("handle error: %s", err.Error())
			}
		}()
	}
	return nil
}

func (server *FtpServer) ListenAndServe() error {
	if err := server.Listen(); err != nil {
		return err
	}

	logrus.Info("Starting...")

	return server.Serve()
}

func (server *FtpServer) Stop() {
	if server.listener != nil {
		server.listener.Close()
	}
}
