package pftp

import (
	"net"
	"strings"

	"github.com/sirupsen/logrus"
)

type middlewareFunc func(*Context, string) error
type middleware map[string]middlewareFunc

type FtpServer struct {
	listener      net.Listener
	ClientCounter uint32
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
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			if server.listener != nil {
				return err
			}
		}

		server.clientArrival(connection)
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

func (server *FtpServer) clientArrival(conn net.Conn) error {
	c := server.newClientHandler(conn, server.config, server.middleware)
	go c.HandleCommands()

	logrus.Info("FTP Client connected ", "clientIp ", conn.RemoteAddr())

	return nil
}
