package pftp

import (
	"net"
	"strings"
	"testing"

	"github.com/pyama86/pftp/test"
)

func Test_clientHandler_HandleCommands(t *testing.T) {
	var server net.Listener
	serverready := make(chan struct{})
	conn := make(chan net.Conn)
	done := make(chan struct{})

	go func() {
		server = test.LaunchTestServer(t)
		defer server.Close()

		serverready <- struct{}{}
		for {
			c, err := server.Accept()
			if err != nil {
				if strings.Index(err.Error(), "use of closed network connection") == -1 {
					t.Fatal(err)
				}
				break
			}

			conn <- c
		}
		done <- struct{}{}
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
	<-serverready

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			clientHandler := newClientHandler(
				<-conn,
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
	var server net.Listener
	conn := make(chan net.Conn)
	done := make(chan struct{})
	serverready := make(chan struct{})

	go func() {
		server = test.LaunchTestServer(t)
		defer server.Close()

		serverready <- struct{}{}
		for {
			c, err := server.Accept()
			if err != nil {
				if strings.Index(err.Error(), "use of closed network connection") == -1 {
					t.Fatal(err)
				}
				break
			}

			conn <- c
		}
		done <- struct{}{}
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
		wantR  *result
	}{
		{
			name: "ok",
			fields: fields{
				config: &config{
					IdleTimeout: 3,
					RemoteAddr:  "127.0.0.1:21",
				},
			},
			args: args{
				line: "user pftp",
			},
		},
		{
			name: "not connect",
			fields: fields{
				config: &config{
					IdleTimeout: 3,
					RemoteAddr:  "127.0.0.1:28080",
				},
			},
			args: args{
				line: "user pftp",
			},
			wantR: &result{
				code: 530,
				msg:  "I can't deal with you (proxy error)",
			},
		},
	}

	<-serverready

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			clientHandler := newClientHandler(
				<-conn,
				tt.fields.config,
				nil,
			)

			got := clientHandler.handleCommand(tt.args.line)
			if (got != nil && tt.wantR == nil) || (tt.wantR != nil && (got.code != tt.wantR.code || got.msg != tt.wantR.msg)) {
				t.Errorf("clientHandler.handleCommand() = %v, want %v", got, tt.wantR)
			}
		})
	}

	server.Close()
	<-done
}
