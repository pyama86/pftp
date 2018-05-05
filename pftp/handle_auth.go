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

	// read welcome message
	_, err = p.ReadFromOrigin()
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
	p.SendToOriginWithProxy(c.line)
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
