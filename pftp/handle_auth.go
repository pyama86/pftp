package pftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
)

func (c *clientHandler) handleUSER() *result {
	p, err := NewProxyServer(c.config.ProxyTimeout, c.conn, c.context.RemoteAddr)
	if err != nil {
		return &result{
			code: 530,
			msg:  "I can't deal with you (proxy error)",
			err:  err,
		}
	}

	if c.controleProxy != nil {
		c.controleProxy.Close()
	}

	c.controleProxy = p

	// read welcome message
	if _, err := p.ReadFromOrigin(); err != nil {
		return &result{
			code: 530,
			msg:  "I can't deal with you (proxy error)",
			err:  err,
		}
	}

	if err := p.SendToOrigin(c.line); err != nil {
		return &result{
			code: 530,
			msg:  "I can't deal with you (proxy error)",
			err:  err,
		}
	}
	return nil
}

func (c *clientHandler) handleAUTH() *result {
	if c.config.TLSConfig != nil {
		c.conn = tls.Server(c.conn, c.config.TLSConfig)
		c.reader = bufio.NewReader(c.conn)
		c.writer = bufio.NewWriter(c.conn)
		return &result{
			code: 234,
			msg:  "AUTH command ok. Expecting TLS Negotiation.",
		}
	}
	return &result{
		code: 550,
		msg:  fmt.Sprint("Cannot get a TLS config"),
	}
}
