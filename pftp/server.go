package pftp

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pyama86/ftpserver/server"
	"github.com/go-kit/kit/log"
)

var ftpServer *server.FtpServer

type PFTPServer struct {
	confPath       string
	AuthMiddleware Auther
}

func NewServer(confPath string) *PFTPServer {
	return &PFTPServer{
		confPath: confPath,
	}
}

/*
	Auth(user_name, password) (baseDir, error)
*/
type Auther interface {
	Auth(string, string) (string, error)
}

func (p *PFTPServer) Start() error {
	logger := log.With(
		log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		"ts", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
	)
	driver, err := NewDriver(p.confPath)
	if err != nil {
		return err
	}

	// set drivers
	driver.AuthMiddleware = p.AuthMiddleware
	driver.Logger = log.With(logger, "component", "driver")

	ftpServer = server.NewFtpServer(driver)

	ftpServer.Logger = log.With(logger, "component", "server")
	go signalHandler()
	if err := ftpServer.ListenAndServe(); err != nil {
		return err
	}
	return nil
}

func signalHandler() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGTERM)
	for {
		switch <-ch {
		case syscall.SIGTERM:
			ftpServer.Stop()
			break
		}
	}
}
