package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
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
		config *config
	}
	type args struct {
		line string
	}
	type want struct {
		result *result
		srcIP  string
	}

	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "user_ok",
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
			name: "proxy_invalid_proxyheader",
			fields: fields{
				config: &config{
					IdleTimeout: 5,
					RemoteAddr:  "127.0.0.1:21",
				},
			},
			args: args{
				line: "PROXY 192.168.10.1 100.100.100.100 53172 21\r\n",
			},
			want: want{
				result: &result{
					code: 500,
					msg:  "Proxy header parse error",
					err:  errors.New("wrong proxy header parameters"),
				},
			},
		},
		{
			name: "proxy_invalid_source_ip",
			fields: fields{
				config: &config{
					IdleTimeout: 5,
					RemoteAddr:  "127.0.0.1:21",
				},
			},
			args: args{
				line: "PROXY TCP4 192.168.10.256 100.100.100.100 53172 21\r\n",
			},
			want: want{
				result: &result{
					code: 500,
					msg:  "Proxy header parse error",
					err:  errors.New("wrong source ip address"),
				},
			},
		},
		{
			name: "proxy_ok",
			fields: fields{
				config: &config{
					IdleTimeout: 5,
					RemoteAddr:  "127.0.0.1:21",
				},
			},
			args: args{
				line: "PROXY TCP4 192.168.10.1 100.100.100.100 12345 21\r\n",
			},
			want: want{
				srcIP: "192.168.10.1:12345",
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
				nil,
				1,
				&cn,
			)

			got := clientHandler.handleCommand(tt.args.line)
			if (got != nil && tt.want.result == nil) || (tt.want.result != nil && (got.code != tt.want.result.code || got.msg != tt.want.result.msg || got.err.Error() != tt.want.result.err.Error())) {
				t.Errorf("clientHandler.handleCommand() = %v, want %v", got, tt.want.result)
			} else if tt.name == "proxy_ok" && clientHandler.srcIP != tt.want.srcIP {
				t.Errorf("clientHandler.sourceIP = %v, want %v", clientHandler.srcIP, tt.want.srcIP)
			}
		})
	}

	server.Close()
	<-done
}

func Test_clientHandler_TLS_error_type_bug(t *testing.T) {
	var server net.Listener
	serverready := make(chan struct{})
	conn := make(chan net.Conn)
	done := make(chan struct{})
	handlerDone := make(chan error)

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
			name: "tls_err_type_check",
			fields: fields{
				config: &config{
					IdleTimeout:    1,
					MaxConnections: 5,
					RemoteAddr:     "127.0.0.1:21",
					WelcomeMsg:     "TLS test server",
					TLS: &tlsPair{
						Cert: "../tls/server.crt",
						Key:  "../tls/server.key",
					},
				},
			},
			hook:    func() { time.Sleep(2 * time.Second) },
			wantErr: false,
		},
	}
	<-serverready

	var cn int32

	for _, tt := range tests {
		tlsData := buildTLSConfigForOrigin()

		t.Run(tt.name, func(t *testing.T) {
			serverTLSConfig, err := buildTLSConfigForClient(tt.fields.config.TLS)
			if err != nil {
				t.Fatal(err)
			}
			// run client handler
			go func() {
				clientHandler := newClientHandler(
					<-conn,
					tt.fields.config,
					serverTLSConfig,
					nil,
					1,
					&cn,
				)

				err := clientHandler.handleCommands()
				handlerDone <- err
			}()

			// connect to test server
			c, err := net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			reader := bufio.NewReader(c)
			writer := bufio.NewWriter(c)

			// read welcome message
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_error_type_bug() can not read welcome message from origin: %v", err)
			}

			// send AUTH command to server
			if _, err = writer.WriteString("AUTH TLS\r\n"); err != nil {
				t.Errorf("clientHandler.TLS_error_type_bug() can not write string to proxy: %v", err)
			}
			if err = writer.Flush(); err != nil {
				t.Errorf("clientHandler.TLS_error_type_bug() can not write string to proxy: %v", err)
			}
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_error_type_bug() can not read response from origin: %v", err)
			}

			// make tls handshake for full tls connection
			tlsConn := tls.Client(c, tlsData.config)
			err = tlsConn.Handshake()
			if err != nil {
				t.Errorf("TLS Handshake Error: %v", err)
			}

			// comment out tls wrapping client connection
			// reader = bufio.NewReader(tlsConn)
			// writer = bufio.NewWriter(tlsConn)

			// send some command using by non wrapped conn
			if _, err = writer.WriteString("NOOP\r\n"); err != nil {
				t.Errorf("clientHandler.TLS_error_type_bug() can not write string to proxy: %v", err)
			}
			if err = writer.Flush(); err != nil {
				t.Errorf("clientHandler.TLS_error_type_bug() can not write string to proxy: %v", err)
			}

			// if err is nil, it means failed on test
			_, err = reader.ReadString('\n')
			if err == nil {
				t.Errorf("clientHandler.TLS_error_type_bug() test failed! successfully read response from origin: %v, want err != nil", err)
			}

			// check connection normally finished
			serverErr := <-handlerDone
			if (serverErr != nil) != tt.wantErr {
				t.Errorf("clientHandler.TLS_error_type_bug() error = %v, wantErr %v\n", serverErr, tt.wantErr)
			}
		})
	}
	server.Close()
	<-done
}

func Test_clientHandler_TLS_Session_Resumption(t *testing.T) {
	var server net.Listener
	serverready := make(chan struct{})
	conn := make(chan net.Conn)
	done := make(chan struct{})
	clientHandlerDone := make(chan error)

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
			name: "err_type_check",
			fields: fields{
				config: &config{
					IdleTimeout:    1,
					MaxConnections: 5,
					RemoteAddr:     "127.0.0.1:20021",
					WelcomeMsg:     "TLS test server",
					TLS: &tlsPair{
						Cert: "../tls/server.crt",
						Key:  "../tls/server.key",
					},
				},
			},
			hook:    func() { time.Sleep(2 * time.Second) },
			wantErr: false,
		},
	}
	<-serverready

	var cn int32

	for _, tt := range tests {
		tlsData := buildTLSConfigForOrigin()

		t.Run(tt.name, func(t *testing.T) {
			serverTLSConfig, err := buildTLSConfigForClient(tt.fields.config.TLS)
			if err != nil {
				t.Fatal(err)
			}
			// run 1st client handler
			go func() {
				clientHandler := newClientHandler(
					<-conn,
					tt.fields.config,
					serverTLSConfig,
					nil,
					1,
					&cn,
				)

				err := clientHandler.handleCommands()
				clientHandlerDone <- err
			}()

			// connect to test server
			c, err := net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			reader := bufio.NewReader(c)
			writer := bufio.NewWriter(c)

			// read welcome message
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not read welcome message from origin: %v", err)
			}

			// send AUTH command to server
			if _, err = writer.WriteString("AUTH TLS\r\n"); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			if err = writer.Flush(); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not read response from origin: %v", err)
			}

			// make tls handshake for full tls connection
			tlsConn := tls.Client(c, tlsData.config)
			err = tlsConn.Handshake()
			if err != nil {
				t.Errorf("TLS Handshake Error: %v", err)
			}

			// check 1st TLS connection is resumed. if already resumed(true) in first time, fail
			state := tlsConn.ConnectionState()
			if state.DidResume {
				t.Errorf("clientHandler.TLS_Session_Resumption() 1st TLS session resumption failed: got = %v, want = false", state.DidResume)
			}

			// close 1st connections
			tlsConn.Close()

			// check 1st connection normally finished
			serverErr := <-clientHandlerDone
			if (serverErr != nil) != tt.wantErr {
				t.Errorf("clientHandler.TLS_Session_Resumption() error = %v, wantErr %v\n", serverErr, tt.wantErr)
			}

			// 1st TLS connection is done
			// run 2nd client handler
			go func() {
				clientHandler := newClientHandler(
					<-conn,
					tt.fields.config,
					serverTLSConfig,
					nil,
					1,
					&cn,
				)

				err := clientHandler.handleCommands()
				clientHandlerDone <- err
			}()

			// connect to test server
			c, err = net.Dial("tcp", server.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			reader = bufio.NewReader(c)
			writer = bufio.NewWriter(c)

			// read welcome message
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not read welcome message from origin: %v", err)
			}

			// send AUTH command to server
			if _, err = writer.WriteString("AUTH TLS\r\n"); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			if err = writer.Flush(); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not read response from origin: %v", err)
			}

			// make tls handshake for full tls connection
			tlsConn = tls.Client(c, tlsData.config)
			err = tlsConn.Handshake()
			if err != nil {
				t.Errorf("TLS Handshake Error: %v", err)
			}

			// check 2nd TLS connection is resumed. if not resumed(false), fail
			state = tlsConn.ConnectionState()
			if !state.DidResume {
				t.Errorf("clientHandler.TLS_Session_Resumption() 2nd TLS session resumption failed: got = %v, want = true", state.DidResume)
			}

			reader = bufio.NewReader(tlsConn)
			writer = bufio.NewWriter(tlsConn)

			// test some FTP commands under TLS session
			if _, err = writer.WriteString("FEAT\r\n"); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			if err = writer.Flush(); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not read response from origin: %v", err)
			}

			if _, err = writer.WriteString("QUIT\r\n"); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			if err = writer.Flush(); err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not write string to proxy: %v", err)
			}
			_, err = reader.ReadString('\n')
			if err != nil {
				t.Errorf("clientHandler.TLS_Session_Resumption() can not read response from origin: %v", err)
			}

			tlsConn.Close()

			// wait until 2nd client Handler end
			serverErr = <-clientHandlerDone

			if (serverErr != nil) != tt.wantErr {
				t.Errorf("clientHandler.TLS_Session_Resumption() error = %v, wantErr %v\n", serverErr, tt.wantErr)
			}
		})
	}
	server.Close()
	<-done
}
