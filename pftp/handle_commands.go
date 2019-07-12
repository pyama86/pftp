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
)

func (c *clientHandler) handleUSER() *result {
	// make fail when try to login after logged in
	if c.isLoggedin {
		return &result{
			code: 500,
			msg:  "Already logged in",
			err:  fmt.Errorf("Already logged in"),
			log:  c.log,
		}
	}

	if err := c.connectProxy(); err != nil {
		// user not found
		if err.Error() == "user id not found" {
			return &result{
				code: 530,
				msg:  err.Error(),
				err:  err,
				log:  c.log,
			}
		}

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

func (c *clientHandler) handlePROXY() *result {
	c.readlockMutex.Lock()
	c.readLock = true
	c.readlockMutex.Unlock()

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

// handle PORT, EPRT, PASV, EPSV commands when set data channel proxy is true
func (c *clientHandler) handleDATA() *result {
	// if data channel proxy used
	if c.config.DataChanProxy {
		var toOriginMsg string

		// make new listener and store listener port
		dataHandler, err := newDataHandler(
			c.config,
			c.log,
			c.conn,
			c.proxy.GetConn(),
			c.command,
			&c.inDataTransfer,
		)
		if err != nil {
			return &result{
				code: 421,
				msg:  "cannot create data channel socket",
				err:  err,
				log:  c.log,
			}
		}

		c.proxy.SetDataHandler(dataHandler)

		switch c.command {
		case "PORT":
			if err := c.proxy.dataConnector.parsePORTcommand(c.line); err != nil {
				return &result{
					code: 501,
					msg:  "cannot parse PORT command",
					err:  err,
					log:  c.log,
				}
			}
			break
		case "EPRT":
			if err := c.proxy.dataConnector.parseEPRTcommand(c.line); err != nil {
				if err.Error() == "unknown network protocol" {
					return &result{
						code: 522,
						msg:  err.Error(),
						err:  err,
						log:  c.log,
					}
				}

				return &result{
					code: 501,
					msg:  "cannot parse EPRT command",
					err:  err,
					log:  c.log,
				}
			}
			break
		}

		// if origin connect mode is PORT or CLIENT(with client use some kind of active mode)
		if c.proxy.dataConnector.originConn.needsListen {
			_, lPort, _ := net.SplitHostPort(c.proxy.dataConnector.originConn.listener.Addr().String())
			listenPort, _ := strconv.Atoi(lPort)

			listenIP := strings.Split(c.proxy.GetConn().LocalAddr().String(), ":")[0]

			// prepare PORT command line to origin
			// only use PORT command because connect to server support IPv4 now
			toOriginMsg = fmt.Sprintf("PORT %s,%s,%s\r\n",
				strings.ReplaceAll(listenIP, ".", ","),
				strconv.Itoa(listenPort/256),
				strconv.Itoa(listenPort%256))
		} else {
			if c.config.TransferMode == "CLIENT" {
				toOriginMsg = c.command + "\r\n"
			} else {
				toOriginMsg = c.config.TransferMode + "\r\n"
			}
		}

		// send command to origin
		if err := c.proxy.sendToOrigin(toOriginMsg); err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", err),
				err:  err,
				log:  c.log,
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
