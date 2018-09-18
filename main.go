package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Gurpartap/logrus-stack"
	"github.com/pyama86/pftp/example/webapi"
	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

var ftpServer *pftp.FtpServer

var confFile = "./config.toml"

var serverCh = make(chan *pftp.FtpServer)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	stackLevels := []logrus.Level{logrus.PanicLevel, logrus.FatalLevel}
	logrus.AddHook(logrus_stack.NewHook(stackLevels, stackLevels))
}

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGHUP, syscall.SIGTERM)

	go run()
	ftpServer = <-serverCh

	for {
		switch <-sigCh {
		case syscall.SIGINT:
			break
		case syscall.SIGTERM:
			logrus.Info("SIGTERM recived")
			ftpServer.Stop()
			os.Exit(0)
			break
		case syscall.SIGHUP:
			logrus.Info("SIGHUP recived")
			ftpServer.Stop()

			go run()
			ftpServer = <-serverCh
			break
		}
	}
}

func run() {
	ftpServer, err := pftp.NewFtpServer(confFile)
	if err != nil {
		logrus.Fatal(err)
	}

	serverCh <- ftpServer

	ftpServer.Use("user", User)
	if err := ftpServer.ListenAndServe(); (err != nil) && (!strings.Contains(err.Error(), "use of closed network connection")) {
		logrus.Fatal(err)
	}
}

// User function will setup Origin ftp server domain from ftp username
// If failed get domain from server, the origin will set by local (localhost:21)
func User(c *pftp.Context, param string) error {
	res, err := webapi.GetDomainFromWebAPI(confFile, param)
	if err != nil {
		logrus.Debug(fmt.Sprintf("cannot get domain from webapi server:%v", err))
		c.RemoteAddr = "127.0.0.1:21"
	} else {
		c.RemoteAddr = *res
	}

	return nil
}
