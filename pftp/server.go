package pftp

import (
	"net"

	"github.com/sirupsen/logrus"
)

type FtpServer struct {
	listener      net.Listener
	ClientCounter uint32
	config        *config
	middleware    Middleware
}

func NewFtpServer(confFile string) (*FtpServer, error) {
	c, err := loadConfig(confFile)
	if err != nil {
		return nil, err
	}
	return &FtpServer{
		config: c,
	}, nil
}

func (server *FtpServer) Use(m Middleware) {
	server.middleware = m
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
	server.ClientCounter++
	id := server.ClientCounter

	c := server.newClientHandler(conn, id)
	go c.HandleCommands()

	logrus.Info("FTP Client connected ", "clientIp ", conn.RemoteAddr())

	return nil
}
