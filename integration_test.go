package main

import (
	"flag"
	"os"
	"testing"

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

func TestAuth(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	client := test.LocalConnect(2121, t)
	defer client.Quit()

	err := client.Login("pftp", "pftp")
	if err != nil {
		t.Errorf("integration.TestAuth() error = %v, wantErr %v", err, nil)
	}

}
