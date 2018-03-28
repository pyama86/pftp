package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

var ftpServer *pftp.FtpServer

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}
func main() {
	confFile := "./example.toml"

	ftpServer, err := pftp.NewFtpServer(confFile)
	if err != nil {
		logrus.Fatal(err)
	}
	if err := ftpServer.ListenAndServe(); err != nil {
		logrus.Fatal(err)
	}
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
