package pftp

import (
	"bufio"
	"net"
	"reflect"
	"testing"
)

func Test_clientHandler_TransferOpen(t *testing.T) {
	type fields struct {
		conn          net.Conn
		config        *config
		middleware    middleware
		writer        *bufio.Writer
		reader        *bufio.Reader
		line          string
		command       string
		param         string
		transfer      transferHandler
		transferTLS   bool
		controleProxy *ProxyServer
		context       *Context
	}
	tests := []struct {
		name    string
		fields  fields
		want    *ProxyServer
		wantErr bool
	}{
		{
			name:    "Proxy does not exist",
			fields:  fields{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clientHandler{
				conn:          tt.fields.conn,
				config:        tt.fields.config,
				middleware:    tt.fields.middleware,
				writer:        tt.fields.writer,
				reader:        tt.fields.reader,
				line:          tt.fields.line,
				command:       tt.fields.command,
				param:         tt.fields.param,
				transfer:      tt.fields.transfer,
				transferTLS:   tt.fields.transferTLS,
				controleProxy: tt.fields.controleProxy,
				context:       tt.fields.context,
			}
			got, err := c.TransferOpen()
			if (err != nil) != tt.wantErr {
				t.Errorf("clientHandler.TransferOpen() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("clientHandler.TransferOpen() = %v, want %v", got, tt.want)
			}
		})
	}
}
