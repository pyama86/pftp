package pftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
)

func (c *clientHandler) handleUSER() {
	server, err := c.daddy.middleware.User(c.param)
	if err != nil {
		c.writeMessage(530, "I can't deal with you (proxy error)")
		return
	}

	p, err := NewProxyServer(c.daddy.config.ProxyTimeout, c.conn, server)
	if err != nil {
		c.writeMessage(530, "I can't deal with you (proxy error)")
		return
	}

	// read welcome message
	p.ReadFromOrigin()

	if c.controlProxy != nil {
		c.controlProxy.Close()
	}
	c.controlProxy = p
	p.SendToOriginWithProxy(c.line)
}

func (c *clientHandler) handleAUTH() {
	if c.daddy.config.TLSConfig != nil {
		c.writeMessage(234, "AUTH command ok. Expecting TLS Negotiation.")
		c.conn = tls.Server(c.conn, c.daddy.config.TLSConfig)
		c.reader = bufio.NewReader(c.conn)
		c.writer = bufio.NewWriter(c.conn)
	} else {
		c.writeMessage(550, fmt.Sprint("Cannot get a TLS config"))
	}
}
