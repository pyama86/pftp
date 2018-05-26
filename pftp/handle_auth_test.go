package pftp

import (
	"crypto/tls"
	"net"
	"reflect"
	"testing"

	"github.com/pyama86/pftp/test"
)

func Test_clientHandler_handleAUTH(t *testing.T) {
	cert, _ := test.GetCertificate()

	type fields struct {
		conn   net.Conn
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
				conn:   tt.fields.conn,
				config: tt.fields.config,
			}
			if got := c.handleAUTH(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clientHandler.handleAUTH() = %v, want %v", got, tt.want)
			}
		})
	}
}
