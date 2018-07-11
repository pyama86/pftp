package main

import (
	"errors"
	"flag"
	"os"
	"testing"

	"github.com/marcobeierer/ftps"
	"github.com/pyama86/pftp/test"
)

var (
	integration = flag.Bool("integration", false, "run integration tests")
)

func TestMain(m *testing.M) {
	flag.Parse()
	result := m.Run()
	os.Exit(result)
}

func TestConnect(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := test.LocalConnect(2121, t)
	defer client.Quit()
}

func TestLogin(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := test.LocalConnect(2121, t)
	defer client.Quit()

	err := client.Login("pftp", "pftp")
	if err != nil {
		t.Errorf("integration.TestLogin() error = %v, wantErr %v", err, nil)
	}

}

func TestAuth(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := new(ftps.FTPS)
	client.Debug = true
	client.TLSConfig.InsecureSkipVerify = true

	err := client.Connect("localhost", 2121)
	if err != nil {
		t.Errorf("integration.TestAuth() error = %v, wantErr %v", err, nil)
	}

	err = client.Login("pftp", "pftp")
	if err == nil {
		t.Errorf("integration.TestAuth() error = %v, wantErr %v", err, errors.New("550 Permission denied."))
	}
}
