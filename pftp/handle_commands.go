package pftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

func (c *clientHandler) handleUSER() *result {
	err := c.connectControlProxy()
	if err != nil {
		return &result{
			code: 530,
			msg:  "I can't deal with you (proxy error)",
			err:  err,
			log:  c.log,
		}
	}

	if err := c.controleProxy.SendToOrigin(c.line); err != nil {
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
	logrus.Info("transfer=", c.config.TransferTimeout)
	if c.config.TransferTimeout > 0 {
		c.conn.SetDeadline(time.Now().Add(time.Duration(c.config.TransferTimeout) * time.Second))
	}
	if err := c.controleProxy.SendToOrigin(c.line); err != nil {
		return &result{
			code: 500,
			msg:  fmt.Sprintf("Internal error: %s", err),
		}
	}
	return nil
}
