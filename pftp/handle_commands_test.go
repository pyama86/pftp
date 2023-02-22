package pftp

import (
	"net"
	"reflect"
	"testing"

	"github.com/tevino/abool"
)

func Test_clientHandler_handleAUTH(t *testing.T) {
	type fields struct {
		config *config
	}

	type res struct {
		code int
		msg  string
		err  string
		log  *logger
	}

	tests := []struct {
		name   string
		fields fields
		want   *res
	}{
		{
			name: "undefined",
			fields: fields{
				config: &config{},
			},
			want: &res{
				code: 550,
				msg:  "Cannot get a TLS config",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				config: tt.fields.config,
				tlsDatas: &tlsDataSet{
					forOrigin: &tlsData{},
					forClient: &tlsData{},
				},
			}
			r := c.handleAUTH()
			got := &res{
				code: r.code,
				msg:  r.msg,
				log:  r.log,
			}
			if r.err == nil {
				got.err = ""
			} else {
				got.err = r.err.Error()
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clientHandler.handleAUTH() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clientHandler_handlePBSZ(t *testing.T) {
	type fields struct {
		config *config
	}

	type res struct {
		code int
		msg  string
		err  string
		log  *logger
	}

	tests := []struct {
		name   string
		fields fields
		want   *res
	}{
		{
			name: "none_tls",
			fields: fields{
				config: &config{},
			},
			want: &res{
				code: 503,
				msg:  "Not using TLS connection",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				config:       tt.fields.config,
				controlInTLS: abool.New(),
			}
			r := c.handlePBSZ()
			got := &res{
				code: r.code,
				msg:  r.msg,
				log:  r.log,
			}
			if r.err == nil {
				got.err = ""
			} else {
				got.err = r.err.Error()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clientHandler.handlePBSZ() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clientHandler_handlePROT(t *testing.T) {
	type fields struct {
		config *config
	}

	type res struct {
		code int
		msg  string
		err  string
		log  *logger
	}

	tests := []struct {
		name   string
		fields fields
		want   *res
	}{
		{
			name: "none_tls",
			fields: fields{
				config: &config{},
			},
			want: &res{
				code: 503,
				msg:  "Not using TLS connection",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				config:       tt.fields.config,
				controlInTLS: abool.New(),
			}
			r := c.handlePBSZ()
			got := &res{
				code: r.code,
				msg:  r.msg,
				log:  r.log,
			}
			if r.err == nil {
				got.err = ""
			} else {
				got.err = r.err.Error()
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clientHandler.handlePROT() = %v, want %v", got, tt.want)
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
					RemoteAddr: "127.0.0.1:28080",
				},
				line: "user pftp",
			},
			want: &result{
				code: 530,
				msg:  "I can't deal with you (proxy error for user)",
			},
		},
	}
	var cn int32
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				config:            tt.fields.config,
				conn:              tt.fields.conn,
				context:           tt.fields.context,
				log:               &logger{},
				currentConnection: &cn,
				line:              tt.fields.line,
			}
			got := c.handleUSER()
			if (got != nil && tt.want == nil) || (tt.want != nil && (got.code != tt.want.code || got.msg != tt.want.msg)) {
				t.Errorf("clientHandler.handleUSER() = %v, want %v", got, tt.want)
			}
		})
	}
}
