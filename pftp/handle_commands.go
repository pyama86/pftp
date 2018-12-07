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
	c.isLoggedin = true

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
		c.isTLS = true

		return nil
	}
	return &result{
		code: 550,
		msg:  fmt.Sprint("Cannot get a TLS config"),
	}
}

// response PBSZ to client and store command line when connect by TLS & not loggined
func (c *clientHandler) handlePBSZ() *result {
	if c.isTLS {
		if !c.isLoggedin {
			var r *result
			r = &result{
				code: 200,
				msg:  fmt.Sprintf("PBSZ %s successful", c.param),
			}

			if err := r.Response(c); err != nil {
				return &result{
					code: 550,
					msg:  fmt.Sprint("Client Response Error"),
					err:  err,
					log:  c.log,
				}
			}
			c.previousTLSCommands = append(c.previousTLSCommands, c.line)
		} else {
			if err := c.proxy.sendToOrigin(c.line); err != nil {
				return &result{
					code: 530,
					msg:  "I can't deal with you (proxy error)",
					err:  err,
					log:  c.log,
				}
			}
		}
		return nil
	}
	return &result{
		code: 503,
		msg:  fmt.Sprint("Not using TLS connection"),
	}
}

// response PROT to client and store command line when connect by TLS & not loggined
func (c *clientHandler) handlePROT() *result {
	if c.isTLS {
		if !c.isLoggedin {
			var r *result
			if c.param == "C" {
				r = &result{
					code: 200,
					msg:  "Protection Set to Clear",
				}
			} else if c.param == "P" {
				r = &result{
					code: 200,
					msg:  "Protection Set to Private.",
				}
			} else {
				r = &result{
					code: 534,
					msg:  "Only C or P Level supported.",
				}
			}

			if err := r.Response(c); err != nil {
				return &result{
					code: 550,
					msg:  fmt.Sprint("Client Response Error"),
					err:  err,
					log:  c.log,
				}
			}
			c.previousTLSCommands = append(c.previousTLSCommands, c.line)
		} else {
			if err := c.proxy.sendToOrigin(c.line); err != nil {
				return &result{
					code: 530,
					msg:  "I can't deal with you (proxy error)",
					err:  err,
					log:  c.log,
				}
			}
		}

		return nil
	}
	return &result{
		code: 503,
		msg:  fmt.Sprint("Not using TLS connection"),
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

	c.srcIP = params[2] + ":" + params[4]

	return nil
}
