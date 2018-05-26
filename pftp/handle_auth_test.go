package pftp

import (
	"crypto/tls"
	"net"
	"reflect"
	"testing"

	"github.com/k0kubun/pp"
	"github.com/pyama86/pftp/test"
)

func Test_clientHandler_handleAUTH(t *testing.T) {
	cert, _ := test.GetCertificate()

	type fields struct {
		config *config
	}
	tests := []struct {
		name   string
		fields fields
		want   *result
	}{
		{
			name: "ok",
			fields: fields{
				config: &config{
					TLSConfig: &tls.Config{
						NextProtos:   []string{"ftp"},
						Certificates: []tls.Certificate{*cert},
					},
				},
			},
			want: &result{
				code: 234,
				msg:  "AUTH command ok. Expecting TLS Negotiation.",
			},
		},
		{
			name: "undefined",
			fields: fields{
				config: &config{},
			},
			want: &result{
				code: 550,
				msg:  "Cannot get a TLS config",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				config: tt.fields.config,
			}
			if got := c.handleAUTH(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clientHandler.handleAUTH() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clientHandler_handleUSER(t *testing.T) {
	c, err := net.Dial("tcp", "127.0.0.1:21")
	if err != nil {
		panic(err)
	}
	defer c.Close()

	type fields struct {
		conn    net.Conn
		config  *config
		context *Context
		line    string
	}
	tests := []struct {
		name   string
		fields fields
		want   *result
	}{
		{
			name: "ok",
			fields: fields{
				conn:   c,
				config: &config{},
				context: &Context{
					RemoteAddr: "127.0.0.1:21",
				},
				line: "user pftp",
			},
		},
		{
			name: "not connect",
			fields: fields{
				conn:   c,
				config: &config{},
				context: &Context{
					RemoteAddr: "127.0.0.1:22",
				},
				line: "user pftp",
			},
			want: &result{
				code: 530,
				msg:  "I can't deal with you (proxy error)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				config:  tt.fields.config,
				conn:    tt.fields.conn,
				context: tt.fields.context,
			}
			got := c.handleUSER()
			pp.Println(got)
			pp.Println(tt.want)
			if (got != nil && tt.want == nil) || (tt.want != nil && (got.code != tt.want.code || got.msg != tt.want.msg)) {
				t.Errorf("clientHandler.handleUSER() = %v, want %v", got, tt.want)
			}
		})
	}
}
