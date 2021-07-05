package pftp

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
)

func (c *clientHandler) handleUSER() *result {
	// make fail when try to login after logged in
	if c.proxy != nil {
		if c.proxy.isLoggedIn() {
			return &result{
				code: 500,
				msg:  "Already logged in",
				err:  fmt.Errorf("already logged in"),
				log:  c.log,
			}
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

		// if proxy server attached, change proxy handler's client reader & writer to TLS conn
		if c.proxy != nil {
			c.proxy.clientReader = c.reader
			c.proxy.clientWriter = c.writer
		}

		c.previousTLSCommands = append(c.previousTLSCommands, c.line)

		atomic.StoreInt32(&c.controlInTLS, 1)

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
	if atomic.LoadInt32(&c.controlInTLS) == 1 {
		if !c.proxy.isLoggedIn() {
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
	if atomic.LoadInt32(&c.controlInTLS) == 1 {
		if !c.proxy.isLoggedIn() {
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

		if c.param == "P" {
			atomic.StoreInt32(&c.transferInTLS, 1)
		} else {
			atomic.StoreInt32(&c.transferInTLS, 0)
		}

		return nil
	}
	return &result{
		code: 503,
		msg:  "Not using TLS connection",
	}
}

func (c *clientHandler) handleTransfer() *result {
	if !c.proxy.isLoggedIn() {
		return &result{
			code: 530,
			msg:  "Please login with USER and PASS",
		}
	}

	if !c.proxy.isDataHandlerAvailable() {
		return &result{
			code: 425,
			msg:  "Can't open data connection",
		}
	}

	if c.proxy.isDataTransferStarted() {
		return &result{
			code: 450,
			msg:  fmt.Sprintf("%s: data transfer in progress", c.command),
		}
	}

	// set transfer in progress flag to 1
	atomic.StoreInt32(&c.inDataTransfer, 1)

	// start data transfer by direction
	switch c.command {
	case "RETR", "LIST", "MLSD", "NLST":
		// set transfer direction to download
		go c.proxy.dataConnector.StartDataTransfer(downloadStream)
	case "STOR", "STOU", "APPE":
		// set transfer direction to upload
		go c.proxy.dataConnector.StartDataTransfer(uploadStream)
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
	if !c.proxy.isLoggedIn() {
		return &result{
			code: 530,
			msg:  "Please login with USER and PASS",
		}
	}

	// if data channel proxy used
	if c.config.DataChanProxy {
		var toOriginMsg string

		// only one data connection available in same time.
		// Return 450 response code to client without create
		// & attach new data handler when data transfer in progress.
		if c.proxy.isDataTransferStarted() {
			return &result{
				code: 450,
				msg:  fmt.Sprintf("%s: data transfer in progress", c.command),
			}
		}

		// make new listener and store listener port
		dataHandler, err := newDataHandler(
			c.config,
			c.log,
			c.conn,
			c.proxy.GetConn(),
			c.command,
			c.tlsDatas,
			&c.transferInTLS,
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

		if !c.proxy.isDataHandlerAvailable() {
			return &result{
				code: 425,
				msg:  "Can't open data connection",
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
