package pftp

import (
	"net"
	"strings"
	"testing"

	"github.com/pyama86/pftp/test"
)

func Test_clientHandler_HandleCommands(t *testing.T) {
	var conn net.Conn
	var server net.Listener
	done := make(chan struct{})
	serverready := make(chan struct{})
	clientready := make(chan struct{})

	go func() {
		server = test.LaunchTestServer(t)
		defer server.Close()

		serverready <- struct{}{}
		for {
			c, err := server.Accept()
			if err != nil {
				done <- struct{}{}
				if strings.Index(err.Error(), "use of closed network connection") == -1 {
					t.Fatal(err)
				}
				break
			}

			conn = c
			clientready <- struct{}{}
		}
	}()

	type fields struct {
		config *config
	}

	tests := []struct {
		name    string
		fields  fields
		command string
	}{
		{
			name: "user ok",
			fields: fields{
				config: &config{
					IdleTimeout: 3,
				},
			},
			command: "user pftp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			<-serverready
			c, err := net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			<-clientready
			clientHandler := newClientHandler(
				conn,
				tt.fields.config,
				nil,
			)

			c.Write([]byte(tt.command))

			clientHandler.HandleCommands()
		})
	}

	server.Close()
	<-done
}

func Test_clientHandler_handleCommand(t *testing.T) {
	var conn net.Conn
	var server net.Listener
	done := make(chan struct{})
	serverready := make(chan struct{})
	clientready := make(chan struct{})

	go func() {
		server = test.LaunchTestServer(t)
		defer server.Close()

		serverready <- struct{}{}
		for {
			c, err := server.Accept()
			if err != nil {
				done <- struct{}{}
				if strings.Index(err.Error(), "use of closed network connection") == -1 {
					t.Fatal(err)
				}
				break
			}

			conn = c
			clientready <- struct{}{}
		}
	}()

	type fields struct {
		conn   net.Conn
		config *config
	}
	type args struct {
		line string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "ok",
			fields: fields{
				config: &config{
					IdleTimeout: 3,
				},
			},
			args: args{
				line: "user pftp",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			<-serverready
			c, err := net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			<-clientready
			clientHandler := newClientHandler(
				conn,
				tt.fields.config,
				nil,
			)

			clientHandler.handleCommand(tt.args.line)
		})
	}
}
