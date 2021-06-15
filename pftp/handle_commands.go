package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func (c *clientHandler) handleUSER() *result {
	// make fail when try to login after logged in
	if c.isLoggedin {
		return &result{
			code: 500,
			msg:  "Already logged in",
			err:  fmt.Errorf("already logged in"),
			log:  c.log,
		}
	}

	c.log.user = c.param

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

	// unsuspend proxy before send command to origin
	c.proxy.unsuspend()

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
	if c.tlsDatas.forClient.getTLSConfig() != nil {
		r := &result{
			code: 234,
			msg:  fmt.Sprintf("AUTH command ok. Expecting %s Negotiation.", c.param),
		}

		if err := r.Response(c); err != nil {
			return &result{
				code: 550,
				msg:  "Client Response Error",
				err:  err,
				log:  c.log,
			}
		}

		tlsConn := tls.Server(c.conn, c.tlsDatas.forClient.getTLSConfig())
		err := tlsConn.Handshake()
		if err != nil {
			return &result{
				code: 550,
				msg:  "TLS Handshake Error",
				err:  err,
				log:  c.log,
			}
		}

		c.log.debug("TLS control connection finished with client. TLS protocol version: %s and Cipher Suite: %s", getTLSProtocolName(tlsConn.ConnectionState().Version), tls.CipherSuiteName(tlsConn.ConnectionState().CipherSuite))

		c.conn = tlsConn
		c.reader = bufio.NewReader(c.conn)
		c.writer = bufio.NewWriter(c.conn)
		c.previousTLSCommands = append(c.previousTLSCommands, c.line)

		c.controlInTLS = true

		c.tlsDatas.serverName = tlsConn.ConnectionState().ServerName
		c.tlsDatas.version = tlsConn.ConnectionState().Version
		c.tlsDatas.cipherSuite = tlsConn.ConnectionState().CipherSuite

		// set specific client TLS informations to origin TLS config
		c.tlsDatas.forOrigin.setServerName(c.tlsDatas.serverName)
		c.tlsDatas.forOrigin.setSpecificTLSVersion(c.tlsDatas.version)
		c.tlsDatas.forOrigin.setCipherSUite(c.tlsDatas.cipherSuite)

		return nil
	}
	return &result{
		code: 550,
		msg:  "Cannot get a TLS config",
	}
}

// response PBSZ to client and store command line when connect by TLS & not loggined
func (c *clientHandler) handlePBSZ() *result {
	if c.controlInTLS {
		if !c.isLoggedin {
			r := &result{
				code: 200,
				msg:  fmt.Sprintf("PBSZ %s successful", c.param),
			}

			if err := r.Response(c); err != nil {
				return &result{
					code: 550,
					msg:  "Client Response Error",
					err:  err,
					log:  c.log,
				}
			}

			c.previousTLSCommands = append(c.previousTLSCommands, c.line)
		} else {
			// unsuspend proxy before send command to origin
			c.proxy.unsuspend()

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
		msg:  "Not using TLS connection",
	}
}

// response PROT to client and store command line when connect by TLS & not loggined
func (c *clientHandler) handlePROT() *result {
	if c.controlInTLS {
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
					msg:  "Client Response Error",
					err:  err,
					log:  c.log,
				}
			}

			c.previousTLSCommands = append(c.previousTLSCommands, c.line)

		} else {
			// unsuspend proxy before send command to origin
			c.proxy.unsuspend()

			if err := c.proxy.sendToOrigin(c.line); err != nil {
				return &result{
					code: 530,
					msg:  "I can't deal with you (proxy error)",
					err:  err,
					log:  c.log,
				}
			}
		}

		c.transferInTLS = (c.param == "P")

		return nil
	}
	return &result{
		code: 503,
		msg:  "Not using TLS connection",
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
	params := strings.SplitN(strings.Trim(c.line, "\r\n"), " ", 6)
	if len(params) != 6 {
		return &result{
			code: 500,
			msg:  "Proxy header parse error",
			err:  errors.New("wrong proxy header parameters"),
		}
	}

	if net.ParseIP(params[2]) == nil || net.ParseIP(params[3]) == nil {
		return &result{
			code: 500,
			msg:  "Proxy header parse error",
			err:  errors.New("wrong source ip address"),
		}
	}

	c.srcIP = params[2] + ":" + params[4]

	return nil
}

// handle PORT, EPRT, PASV, EPSV commands when set data channel proxy is true
func (c *clientHandler) handleDATA() *result {
	if !c.isLoggedin {
		return &result{
			code: 530,
			msg:  "Please login with USER and PASS",
		}
	}

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
			c.tlsDatas,
			c.transferInTLS,
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
				c.log.err(err.Error())

				c.proxy.dataConnector.Close()

				return &result{
					code: 501,
					msg:  "cannot parse PORT command",
					err:  err,
					log:  c.log,
				}
			}
		case "EPRT":
			if err := c.proxy.dataConnector.parseEPRTcommand(c.line); err != nil {
				c.log.err(err.Error())

				c.proxy.dataConnector.Close()

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
			c.log.err(err.Error())

			c.proxy.dataConnector.Close()

			return &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", err),
				err:  err,
				log:  c.log,
			}
		}
	} else {
		if err := c.proxy.sendToOrigin(c.line); err != nil {
			c.log.err(err.Error())

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
