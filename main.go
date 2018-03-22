package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pyama86/ftpserver/server"
	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

var ftpServer *server.FtpServer

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}
func main() {

	ftpServer := pftp.NewFtpServer()
	if err := ftpServer.ListenAndServe(); err != nil {
		logrus.Fatal("msg", "Problem listening", "err", err)
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
