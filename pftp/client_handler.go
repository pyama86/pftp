package pftp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tevino/abool"
	"golang.org/x/sync/errgroup"
)

type handleFunc struct {
	f       func(*clientHandler) *result
	suspend bool
}

var handlers map[string]*handleFunc

func init() {
	handlers = make(map[string]*handleFunc)
	handlers["PROXY"] = &handleFunc{(*clientHandler).handlePROXY, false}
	handlers["USER"] = &handleFunc{(*clientHandler).handleUSER, true}
	handlers["AUTH"] = &handleFunc{(*clientHandler).handleAUTH, true}
	handlers["PBSZ"] = &handleFunc{(*clientHandler).handlePBSZ, true}
	handlers["PROT"] = &handleFunc{(*clientHandler).handlePROT, true}
	handlers["PORT"] = &handleFunc{(*clientHandler).handleDATA, false}
	handlers["EPRT"] = &handleFunc{(*clientHandler).handleDATA, false}
	handlers["PASV"] = &handleFunc{(*clientHandler).handleDATA, false}
	handlers["EPSV"] = &handleFunc{(*clientHandler).handleDATA, false}

	// handle data transfer begin commands
	handlers["RETR"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["STOR"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["STOU"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["APPE"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["LIST"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["MLSD"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["NLST"] = &handleFunc{(*clientHandler).handleTransfer, false}
}

type clientHandler struct {
	id                  uint64
	conn                net.Conn
	config              *config
	tlsDatas            *tlsDataSet
	controlInTLS        *abool.AtomicBool
	transferInTLS       *abool.AtomicBool
	middleware          middleware
	writer              *bufio.Writer
	reader              *bufio.Reader
	line                string
	command             string
	param               string
	proxy               *proxyServer
	context             *Context
	currentConnection   *int32
	connCounts          int32
	mutex               *sync.Mutex
	log                 *logger
	srcIP               string
	previousTLSCommands []string
	inDataTransfer      *abool.AtomicBool
}

func newClientHandler(connection net.Conn, c *config, sharedTLSData *tlsData, m middleware, id uint64, currentConnection *int32) *clientHandler {
	p := &clientHandler{
		id:                id,
		conn:              connection,
		config:            c,
		controlInTLS:      abool.New(),
		transferInTLS:     abool.New(),
		middleware:        m,
		writer:            bufio.NewWriter(connection),
		reader:            bufio.NewReader(connection),
		context:           newContext(c),
		currentConnection: currentConnection,
		mutex:             &sync.Mutex{},
		log:               &logger{fromip: connection.RemoteAddr().String(), user: "-", id: id},
		srcIP:             connection.RemoteAddr().String(),
		inDataTransfer:    abool.New(),
	}

	// increase current connection count
	p.connCounts = atomic.AddInt32(p.currentConnection, 1)
	p.log.info("FTP Client connected. clientIP: %s. current connection count: %d", p.conn.RemoteAddr(), p.connCounts)

	// is masquerade IP not setted, set local IP of client connection
	if len(p.config.MasqueradeIP) == 0 {
		p.config.MasqueradeIP = strings.Split(connection.LocalAddr().String(), ":")[0]
	}

	// make TLS configs by shared pftp server conf(for client) and client own conf(for origin)
	p.tlsDatas = &tlsDataSet{
		forClient: sharedTLSData,
		forOrigin: buildTLSConfigForOrigin(),
	}

	return p
}

// Close client connection and check return
func (c *clientHandler) Close() error {
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (c *clientHandler) setClientDeadLine(t int) {
	// do not time out during transfer data
	if c.inDataTransfer.IsSet() {
		c.conn.SetDeadline(time.Time{})
	} else {
		c.conn.SetDeadline(time.Now().Add(time.Duration(t) * time.Second))
	}
}

func (c *clientHandler) handleCommands() error {
	defer func() {
		// decrease current connection count
		c.log.info("FTP Client disconnect. clientIP: %s. current connection count: %d", c.conn.RemoteAddr(), atomic.AddInt32(c.currentConnection, -1))

		// close each connection again
		connectionCloser(c, c.log)
		if c.proxy != nil {
			connectionCloser(c.proxy, c.log)
		}
	}()

	// Check max client. If exceeded, send 530 error to client and disconnect
	if c.connCounts > c.config.MaxConnections {
		err := fmt.Errorf("exceeded client connection limit")
		r := result{
			code: 530,
			msg:  "max client exceeded",
			err:  err,
			log:  c.log,
		}
		if err := r.Response(c); err != nil {
			c.log.err("cannot send response to client")
		}

		return err
	}

	eg := errgroup.Group{}

	err := c.connectProxy()
	if err != nil {
		return err
	}

	// run origin response read routine
	eg.Go(func() error { return c.getResponseFromOrigin() })

	// run client command read routine
	eg.Go(func() error { return c.readClientCommands() })

	// wait until all goroutine has done
	if err = eg.Wait(); err != nil && err == io.EOF {
		c.log.info("client disconnected by error")
	} else {
		c.log.info("client disconnected")
		err = nil
	}

	return err
}

func (c *clientHandler) getResponseFromOrigin() error {
	var err error

	// close origin connection when close goroutine
	defer func() {
		// send EOF to client connection. if fail, close immediately
		c.log.debug("send EOF to client")

		if err := sendEOF(c.conn); err != nil {
			c.log.debug("send EOF to client failed. close connection.")
			connectionCloser(c, c.log)
		}

		// close current proxy connection
		connectionCloser(c.proxy, c.log)
	}()

	// サーバからのレスポンスはSuspendしない限り自動で返却される
	for {
		err = c.proxy.responseProxy()
		if err != nil {
			if err == io.EOF {
				c.log.debug("EOF from origin connection")
				err = nil
			} else {
				if !strings.Contains(err.Error(), alreadyClosedMsg) {
					c.log.debug("error from origin connection: %s", err.Error())
				}
			}

			break
		}

		// wait until switching origin server complate
		if c.proxy.stop {
			if !<-c.proxy.waitSwitching {
				err = fmt.Errorf("switch origin to %s is failed", c.context.RemoteAddr)
				c.log.err(err.Error())

				break
			}
		}
	}

	return err
}

func (c *clientHandler) readClientCommands() error {
	lastError := error(nil)

	// close client connection when close goroutine
	defer func() {
		// send EOF to origin connection. if fail, close immediately
		c.log.debug("send EOF to origin")

		if err := sendEOF(c.proxy.GetConn()); err != nil {
			c.log.debug("send EOF to origin failed. close connection.")
			connectionCloser(c.proxy, c.log)
		}

		// close current client connection
		connectionCloser(c, c.log)
	}()

	for {
		if c.config.IdleTimeout > 0 {
			c.setClientDeadLine(c.config.IdleTimeout)
		}

		line, err := c.reader.ReadString('\n')
		if err != nil {
			lastError = err
			if err == io.EOF {
				c.log.debug("EOF from client connection")
				lastError = nil
			} else if c.command == "QUIT" {
				lastError = nil
			} else {
				switch err := err.(type) {
				case net.Error:
					nErr := net.Error(err)
					if nErr.Timeout() {
						c.conn.SetDeadline(time.Now().Add(time.Minute))
						r := result{
							code: 421,
							msg:  "command timeout : closing control connection",
							err:  err,
							log:  c.log,
						}
						if err := r.Response(c); err != nil {
							lastError = fmt.Errorf("response to client error: %v", err)

							break
						}

						// if timeout, send EOF to client connection for graceful disconnect
						c.log.debug("send EOF to client")

						// if send EOF failed, close immediately
						if err := sendEOF(c.conn); err != nil {
							c.log.debug("send EOF to client failed. try to close connection.")
							connectionCloser(c, c.log)
						}

						continue
					} else {
						c.log.debug("error from client connection: %s", err.Error())
					}

				default:
					c.log.debug("error from client connection: %s", err.Error())
				}
			}

			break
		} else {
			commandResponse := c.handleCommand(line)
			if commandResponse != nil {
				if err = commandResponse.Response(c); err != nil {
					lastError = err
					break
				}
			}
		}
	}

	return lastError
}

func (c *clientHandler) writeLine(line string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, err := c.writer.WriteString(line + "\r\n"); err != nil {
		return err
	}
	if err := c.writer.Flush(); err != nil {
		return err
	}

	c.log.debug("send to client: %s", line)
	return nil
}

func (c *clientHandler) writeMessage(code int, message string) error {
	line := fmt.Sprintf("%d %s", code, message)
	return c.writeLine(line)
}

func (c *clientHandler) handleCommand(line string) (r *result) {
	c.parseLine(line)
	defer func() {
		if r := recover(); r != nil {
			r = &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", r),
			}
		}
	}()

	c.commandLog(line)

	if c.middleware[c.command] != nil {
		if err := c.middleware[c.command](c.context, c.param); err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", err),
			}
		}
	}

	cmd := handlers[c.command]
	if cmd != nil {
		if cmd.suspend {
			c.proxy.suspend()
			defer c.proxy.unsuspend()
		}
		res := cmd.f(c)
		if res != nil {
			return res
		}
	} else {
		if err := c.proxy.sendToOrigin(line); err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", err),
			}
		}
	}

	return nil
}

func (c *clientHandler) connectProxy() error {
	if c.proxy != nil {
		err := c.proxy.switchOrigin(c.srcIP, c.context.RemoteAddr, c.previousTLSCommands)
		if err != nil {
			return err
		}
	} else {
		p, err := newProxyServer(
			&proxyServerConfig{
				clientReader:   c.reader,
				clientWriter:   c.writer,
				tlsDatas:       c.tlsDatas,
				originAddr:     c.context.RemoteAddr,
				mutex:          c.mutex,
				log:            c.log,
				config:         c.config,
				inDataTransfer: c.inDataTransfer,
			})
		if err != nil {
			return err
		}
		c.proxy = p

		if c.config.ProxyProtocol {
			if err := p.sendProxyHeader(c.srcIP, c.context.RemoteAddr); err != nil {
				return err
			}
		}
	}

	return nil
}

// Get command from command line
func getCommand(line string) []string {
	return strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)
}

func (c *clientHandler) parseLine(line string) {
	params := getCommand(line)
	c.line = line
	c.command = strings.ToUpper(params[0])
	if len(params) > 1 {
		c.param = params[1]
	}
}

// Hide parameters from log
func (c *clientHandler) commandLog(line string) {
	if strings.Compare(strings.ToUpper(getCommand(line)[0]), secureCommand) == 0 {
		c.log.info("read from client: %s ********\r\n", secureCommand)
	} else {
		c.log.info("read from client: %s", strings.TrimSuffix(line, "\r\n"))
	}
}
