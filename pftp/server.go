package pftp

import (
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/lestrrat/go-server-starter/listener"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

type middlewareFunc func(*Context, string) error
type middleware map[string]middlewareFunc

// FtpServer struct type
type FtpServer struct {
	listener      net.Listener
	clientCounter int
	config        *config
	middleware    middleware
	shutdown      bool
}

// NewFtpServer load config and create new ftp server struct
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

// Use set middleware function
func (server *FtpServer) Use(command string, m middlewareFunc) {
	server.middleware[strings.ToUpper(command)] = m
}

func (server *FtpServer) listen() (err error) {
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

func (server *FtpServer) serve() error {
	currentConnection := int32(0)
	eg := errgroup.Group{}

	for {
		netConn, err := server.listener.Accept()
		if err != nil {
			// if use server starter, break for while all childs end
			if os.Getenv("SERVER_STARTER_PORT") != "" {
				logrus.Info("Close listener")
				break
			}

			if server.listener != nil {
				return err
			}
		}

		// set linger 0 and tcp keepalive setting between client connection
		conn := netConn.(*net.TCPConn)
		conn.SetKeepAlive(true)
		conn.SetKeepAlivePeriod(time.Duration(server.config.KeepaliveTime) * time.Second)
		conn.SetLinger(0)

		if server.config.IdleTimeout > 0 {
			conn.SetDeadline(time.Now().Add(time.Duration(server.config.IdleTimeout) * time.Second))
		}

		server.clientCounter++

		c := newClientHandler(conn, server.config, server.middleware, server.clientCounter, &currentConnection)
		eg.Go(func() error {
			err := c.handleCommands()
			logrus.Info("handle command end runtime goroutine count: ", runtime.NumGoroutine())
			if err != nil {
				logrus.Error(err.Error())
			}
			return err
		})
	}

	return eg.Wait()
}

// Start start pFTP server
func (server *FtpServer) Start() error {
	var lastError error
	done := make(chan struct{})

	if err := server.listen(); err != nil {
		return err
	}

	logrus.Info("Starting...")

	go func() {
		if err := server.serve(); err != nil {
			if !server.shutdown {
				lastError = err
			}
		}
		done <- struct{}{}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM)
L:
	for {
		switch <-ch {
		case syscall.SIGHUP, syscall.SIGTERM:
			if err := server.stop(); err != nil {
				lastError = err
			}
			break L
		}
	}

	<-done
	return lastError
}

func (server *FtpServer) stop() error {
	server.shutdown = true
	if server.listener != nil {
		if err := server.listener.Close(); err != nil {
			return err
		}
	}
	return nil
}
