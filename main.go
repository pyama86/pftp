package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gurpartap/logrus-stack"
	"github.com/pyama86/pftp/example/webapi"
	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

var ftpServer *pftp.FtpServer

var confFile = "./config.toml"

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	stackLevels := []logrus.Level{logrus.PanicLevel, logrus.FatalLevel}
	logrus.AddHook(logrus_stack.NewHook(stackLevels, stackLevels))
}

func main() {
	done := make(chan struct{})
	ftpServer, err := pftp.NewFtpServer(confFile)
	if err != nil {
		logrus.Fatal(err)
	}

	ftpServer.Use("user", User)
	go func() {
		if err := ftpServer.ListenAndServe(); err != nil {
			logrus.Error(err)
		}
		done <- struct{}{}
	}()

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGHUP)
L:

	for {
		switch <-ch {
		case syscall.SIGHUP, syscall.SIGTERM:
			err := ftpServer.Stop()
			if err != nil {
				logrus.Fatal(err)

			}
			break L
		}
	}
	<-done
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
