package pftp

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/lestrrat-go/server-starter/listener"
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

func setServerStarterPortEnv(starterPort string) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", starterPort)
	if err != nil {
		return err
	}
	tcpListener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}
	sockFile, err := tcpListener.File()
	if err != nil {
		return err
	}

	os.Setenv("SERVER_STARTER_PORT", fmt.Sprintf("%s=%d", starterPort, sockFile.Fd()))

	return nil
}

func (server *FtpServer) Listen() (err error) {
	env := os.Getenv("SERVER_STARTER_PORT")
	pair := strings.Split(env, "=")
	hostAddr := strings.TrimSpace(pair[0])

	if (env == "") || (hostAddr != server.config.ListenAddr) {
		setServerStarterPortEnv(server.config.ListenAddr)
	}

	listeners, err := listener.ListenAll()
	if err != nil && err != listener.ErrNoListeningTarget {
		fmt.Printf("$$$$$$$$$$$$$  at listen all %v\n", err)
		return err
	}

	if len(listeners) > 0 {
		server.listener = listeners[0]
	} else {
		server.listener, err = net.Listen("tcp", server.config.ListenAddr)
		if err != nil {
			return err
		}
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
