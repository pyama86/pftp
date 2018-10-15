package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
)

func (c *clientHandler) handleUSER() *result {
	err := c.connectProxy()
	if err != nil {
		return &result{
			code: 530,
			msg:  "I can't deal with you (proxy error)",
			err:  err,
			log:  c.log,
		}
	}

	if err := c.proxy.sendToOrigin(c.line); err != nil {
		return &result{
			code: 530,
			msg:  "I can't deal with you (proxy error)",
			err:  err,
			log:  c.log,
		}
	}
	return nil
}

func (c *clientHandler) handleAUTH() *result {
	if c.config.TLSConfig != nil && c.param == "TLS" {
		r := &result{
			code: 234,
			msg:  "AUTH command ok. Expecting TLS Negotiation.",
		}

		if err := r.Response(c); err != nil {
			return &result{
				code: 550,
				msg:  fmt.Sprint("Client Response Error"),
				err:  err,
				log:  c.log,
			}
		}

		tlsConn := tls.Server(c.conn, c.config.TLSConfig)
		err := tlsConn.Handshake()
		if err != nil {
			return &result{
				code: 550,
				msg:  fmt.Sprint("TLS Handshake Error"),
				err:  err,
				log:  c.log,
			}
		}

		c.conn = tlsConn
		*c.reader = *(bufio.NewReader(c.conn))
		*c.writer = *(bufio.NewWriter(c.conn))
		return nil
	}
	return &result{
		code: 550,
		msg:  fmt.Sprint("Cannot get a TLS config"),
	}
}

func (c *clientHandler) handleTransfer() *result {
	if c.config.TransferTimeout > 0 {
		c.setClientDeadLine(c.config.TransferTimeout)
	}

	if err := c.proxy.sendToOrigin(c.line); err != nil {
		return &result{
			code: 500,
			msg:  fmt.Sprintf("Internal error: %s", err),
		}
	}
	return nil
}

func (c *clientHandler) handleProxyHeader() *result {
	params := strings.SplitN(strings.Trim(c.line, "\r\n"), " ", 6)
	if len(params) != 6 {
		return &result{
			code: 500,
			msg:  fmt.Sprintf("Proxy header parse error"),
			err:  errors.New("wrong proxy header parameters"),
		}
	}

	if net.ParseIP(params[2]) == nil || net.ParseIP(params[3]) == nil {
		return &result{
			code: 500,
			msg:  fmt.Sprintf("Proxy header parse error"),
			err:  errors.New("wrong source ip address"),
		}
	}

	c.sourceIP = params[2] + ":" + params[4]

	return nil
}
