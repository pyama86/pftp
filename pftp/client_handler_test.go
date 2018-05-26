package pftp

import (
	"bufio"
	"net"
	"testing"
)

func Test_clientHandler_HandleCommands(t *testing.T) {
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
		name   string
		fields fields
	}{
		// TODO: Add test cases.
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
			c.HandleCommands()
		})
	}
}
