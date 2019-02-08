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
)

type handleFunc struct {
	f       func(*clientHandler) *result
	suspend bool
}

var handlers map[string]*handleFunc

func init() {
	handlers = make(map[string]*handleFunc)
	handlers["PROXY"] = &handleFunc{(*clientHandler).handleProxyHeader, false}
	handlers["USER"] = &handleFunc{(*clientHandler).handleUSER, true}
	handlers["AUTH"] = &handleFunc{(*clientHandler).handleAUTH, true}
	handlers["PBSZ"] = &handleFunc{(*clientHandler).handlePBSZ, true}
	handlers["PROT"] = &handleFunc{(*clientHandler).handlePROT, true}
	handlers["RETR"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["STOR"] = &handleFunc{(*clientHandler).handleTransfer, false}
}

type clientHandler struct {
	id                  int
	conn                net.Conn
	config              *config
	middleware          middleware
	writer              *bufio.Writer
	reader              *bufio.Reader
	line                string
	command             string
	param               string
	proxy               *proxyServer
	context             *Context
	currentConnection   *int32
	mutex               *sync.Mutex
	log                 *logger
	deadline            time.Time
	srcIP               string
	tlsProtocol         uint16
	isLoggedin          bool
	previousTLSCommands []string
}

func newClientHandler(connection net.Conn, c *config, m middleware, id int, currentConnection *int32) *clientHandler {
	p := &clientHandler{
		id:                id,
		conn:              connection,
		config:            c,
		middleware:        m,
		writer:            bufio.NewWriter(connection),
		reader:            bufio.NewReader(connection),
		context:           newContext(c),
		currentConnection: currentConnection,
		mutex:             &sync.Mutex{},
		log:               &logger{fromip: connection.RemoteAddr().String(), id: id},
		srcIP:             connection.RemoteAddr().String(),
		tlsProtocol:       0,
		isLoggedin:        false,
	}

	return p
}

func (c *clientHandler) end() {
	c.conn.Close()
	atomic.AddInt32(c.currentConnection, -1)
}

func (c *clientHandler) setClientDeadLine(t int) {
	d := time.Now().Add(time.Duration(t) * time.Second)
	if c.deadline.Unix() < d.Unix() {
		c.deadline = d
		c.conn.SetDeadline(d)
	}
}

func (c *clientHandler) handleCommands() error {
	proxyError := make(chan error)
	clientError := make(chan error)

	err := c.connectProxy()
	if err != nil {
		return err
	}

	defer c.end()

	defer func() {
		close(proxyError)
		close(clientError)

		if c.proxy != nil {
			c.proxy.Close()
		}
	}()

	// run response read routine
	go c.getResponseFromOrigin(proxyError)

	// run command read routine
	go c.readClientCommands(clientError)

	// Check max client. I exceeded, send 530 error to client for disconnect
	if atomic.LoadInt32(c.currentConnection) >= c.config.MaxConnections {
		r := result{
			code: 530,
			msg:  "max client exceeded",
			err:  fmt.Errorf("exceeded client connection limit"),
			log:  c.log,
		}
		if err := r.Response(c); err != nil {
			c.log.err("cannot send response to client")
		}
	}

	// increase current connection
	atomic.AddInt32(c.currentConnection, 1)

	// wait until all goroutine has done
	pError := <-proxyError
	cError := <-clientError

	if c.command == "QUIT" {
		c.log.info("client disconnected by QUIT")
		return nil
	}

	if pError != nil {
		c.log.debug(pError.Error())
	}
	if pError != io.EOF && cError != nil {
		c.log.debug(cError.Error())
	}

	return fmt.Errorf("abnormal end of proxy")
}

func (c *clientHandler) getResponseFromOrigin(proxyError chan error) {
	// サーバからのレスポンスはSuspendしない限り自動で返却される
	for {
		err := c.proxy.responseProxy()
		if err != nil {
			// set client connection timeout immediately for disconnect current connection
			c.conn.SetDeadline(time.Now().Add(0))

			safeSetChanel(proxyError, err)
			return
		}
	}
}

func (c *clientHandler) readClientCommands(clientError chan error) {
	lastError := error(nil)

	for {
		if c.config.IdleTimeout > 0 {
			c.setClientDeadLine(c.config.IdleTimeout)
		}

		line, err := c.reader.ReadString('\n')
		if err != nil {
			// client disconnected by quit command
			if c.command == "QUIT" {
				lastError = nil
				break
			}
			if err == io.EOF {
				if err := c.proxy.sendToOrigin("QUIT"); err != nil {
					c.log.err("got error when send QUIT to origin")
				}

				if err := c.conn.Close(); err != nil {
					c.log.err("got error when close client connection")
				}
				c.log.info("client disconnected by EOF")
				lastError = nil
			} else {
				switch err := err.(type) {
				case net.Error:
					if err.Timeout() {
						c.conn.SetDeadline(time.Now().Add(time.Minute))
						lastError = fmt.Errorf("client disconnected by idle timeout")
						r := result{
							code: 421,
							msg:  "command timeout : closing control connection",
							err:  err,
							log:  c.log,
						}
						if err := r.Response(c); err != nil {
							lastError = fmt.Errorf("response to client error: %v", err)
						}

						// set origin server connection timeout immediately for disconnect current connection
						c.proxy.origin.SetDeadline(time.Now().Add(0))
					}
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

	safeSetChanel(clientError, lastError)
}

func (c *clientHandler) writeLine(line string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if _, err := c.writer.Write([]byte(line)); err != nil {
		return err
	}
	if _, err := c.writer.Write([]byte("\r\n")); err != nil {
		return err
	}
	if err := c.writer.Flush(); err != nil {
		return err
	}
	c.log.debug("send to client:%s", line)
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
			err := c.proxy.suspend()
			if err != nil {
				return &result{
					code: 500,
					msg:  fmt.Sprintf("Internal error: %s", err),
				}
			}
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
		err := c.proxy.switchOrigin(c.srcIP, c.context.RemoteAddr, c.tlsProtocol, c.previousTLSCommands)
		if err != nil {
			return err
		}
	} else {
		p, err := newProxyServer(
			&proxyServerConfig{
				timeout:        c.config.ProxyTimeout,
				clientReader:   c.reader,
				clientWriter:   c.writer,
				originAddr:     c.context.RemoteAddr,
				mutex:          c.mutex,
				log:            c.log,
				proxyProtocol:  c.config.ProxyProtocol,
				secureCommands: c.config.SecureCommands,
			})

		if err != nil {
			return err
		}
		c.proxy = p
	}

	return nil
}

func (c *clientHandler) parseLine(line string) {
	params := strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)
	c.line = line
	c.command = strings.ToUpper(params[0])
	if len(params) > 1 {
		c.param = params[1]
	}
}

// Hide parameters from log
func (c *clientHandler) commandLog(line string) {
	command := strings.ToUpper(strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)[0])
	hideParams := false
	for _, c := range c.config.SecureCommands {
		if strings.Compare(command, c) == 0 {
			hideParams = true
			break
		}
	}

	if hideParams {
		c.log.info("read from client: %s ********", command)
	} else {
		c.log.info("read from client: %s", line)
	}
}
