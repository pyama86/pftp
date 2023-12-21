package pftp

import "net"

// Context struct got remote server address
type Context struct {
	RemoteAddr string
	ClientAddr string
}

func newContext(c *config, conn net.Conn) *Context {
	return &Context{
		RemoteAddr: c.RemoteAddr,
		ClientAddr: conn.RemoteAddr().String(),
	}
}
