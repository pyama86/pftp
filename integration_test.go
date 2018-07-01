package main

import (
	"flag"
	"os"
	"testing"

	"github.com/jlaffaye/ftp"
)

var (
	integration = flag.Bool("integration", false, "run integration tests")
)

func TestMain(m *testing.M) {
	flag.Parse()
	result := m.Run()
	os.Exit(result)
}

func TestConnectOK(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client, err := ftp.Connect("localhost:2121")
	if err != nil {
		panic(err)
	}
	defer client.Quit()
}
