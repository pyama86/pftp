package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
)

func (c *clientHandler) handleUSER() *result {
	if err := c.connectProxy(); err != nil {
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

	// increase current connection after send USER command for real login
	atomic.AddInt32(c.currentConnection, 1)
	c.log.debug("current connection count: %d", atomic.LoadInt32(c.currentConnection))

	return nil
}

func getTLSVersion(c *tls.Conn) uint16 {
	cv := reflect.ValueOf(c)
	switch ce := cv.Elem(); ce.Kind() {
	case reflect.Struct:
		fe := ce.FieldByName("vers")
		return uint16(fe.Uint())
	}
	return 0
}

func (c *clientHandler) handleAUTH() *result {
	if c.config.TLSConfig != nil {
		r := &result{
			code: 234,
			msg:  fmt.Sprintf("AUTH command ok. Expecting %s Negotiation.", c.param),
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
		c.tlsProtocol = getTLSVersion(tlsConn)
		c.previousTLSCommands = append(c.previousTLSCommands, c.line)

		return nil
	}
	return &result{
		code: 550,
		msg:  fmt.Sprint("Cannot get a TLS config"),
	}
}

// response PBSZ to client and store command line when connect by TLS & not loggined
func (c *clientHandler) handlePBSZ() *result {
	if c.tlsProtocol != 0 {
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
	if c.tlsProtocol != 0 {
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

func (c *clientHandler) handlePORT() *result {
	// is data channel proxy used
	if c.config.DataChanProxy {
		localIP := strings.Split(c.proxy.GetConn().LocalAddr().String(), ":")[0]

		// make new listener and store listener port
		listenerIP, listenerPort, err := newDataListener(c.line, localIP, c.config, c.log, "PORT")
		if err != nil {
			return &result{
				code: 421,
				msg:  "cannot create data channel socket",
				err:  err,
				log:  c.log,
			}
		}

		// prepare PORT command line
		line := fmt.Sprintf("PORT %s,%s,%s\r\n",
			strings.ReplaceAll(listenerIP, ".", ","),
			strconv.Itoa(listenerPort/256),
			strconv.Itoa(listenerPort%256))

		// send PORT command to origin
		if err := c.proxy.sendToOrigin(line); err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", err),
			}
		}
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
