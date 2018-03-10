// ftpserver allows to create your own FTP(S) server
package main

import (
	"errors"
	"flag"

	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

type TestAuth struct {
}

func (t TestAuth) Auth(user string, password string) (string, error) {
	if user == "test" && password == "test" {
		return "test", nil
	}

	return "", errors.New("auth error")
}
func main() {
	var confFile, logFile string
	flag.StringVar(&confFile, "conf", "", "Configuration file")
	flag.StringVar(&logFile, "log", "", "Log file")
	flag.Parse()

	ftp := pftp.NewServer(confFile)
	ftp.AuthMiddleware = TestAuth{}
	if err := ftp.Start(); err != nil {
		logrus.Fatal(err)
	}
}
