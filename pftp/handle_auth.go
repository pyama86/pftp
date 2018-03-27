package pftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
)

func (c *clientHandler) handleUSER() {
	p, err := NewProxyServer(c.daddy.config.ProxyTimeout, c.conn, "localhost:2321")
	if err != nil {
		c.writeMessage(530, "I can't deal with you (proxy error)")
		return
	}

	// read welcome message
	p.ReadFromOrigin()
	c.controlProxy = p
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
