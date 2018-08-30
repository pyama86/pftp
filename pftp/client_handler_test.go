package pftp

import (
	"net"
	"testing"
	"time"

	"github.com/pyama86/pftp/test"
)

func Test_clientHandler_handleCommands(t *testing.T) {
	var server net.Listener
	serverready := make(chan struct{})
	conn := make(chan net.Conn)
	done := make(chan struct{})

	go test.LaunchTestServer(&server, conn, done, serverready, t)

	type fields struct {
		config *config
	}

	tests := []struct {
		name    string
		fields  fields
		command string
		hook    func()
		wantErr bool
	}{
		{
			name: "idle_timeout",
			fields: fields{
				config: &config{
					IdleTimeout: 1,
					RemoteAddr:  "127.0.0.1:21",
				},
			},
			hook:    func() { time.Sleep(2 * time.Second) },
			wantErr: true,
		},
		{
			name: "max_connection",
			fields: fields{
				config: &config{
					IdleTimeout:    1,
					MaxConnections: 0,
					RemoteAddr:     "127.0.0.1:21",
				},
			},
			wantErr: true,
		},
	}
	<-serverready
	var cn int32
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
				1,
				&cn,
			)

			if tt.hook != nil {
				tt.hook()
			}

			err = clientHandler.handleCommands()
			if (err != nil) != tt.wantErr {
				t.Errorf("clientHandler.handleCommands() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
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

	go test.LaunchTestServer(&server, conn, done, serverready, t)

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
	}

	<-serverready

	var cn int32
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
				1,
				&cn,
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
