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
	handlers["PORT"] = &handleFunc{(*clientHandler).handleDATA, true}
	handlers["EPRT"] = &handleFunc{(*clientHandler).handleDATA, true}
	handlers["PASV"] = &handleFunc{(*clientHandler).handleDATA, true}
	handlers["EPSV"] = &handleFunc{(*clientHandler).handleDATA, true}
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
	readlockMutex       *sync.Mutex
	log                 *logger
	deadline            time.Time
	srcIP               string
	tlsProtocol         uint16
	isLoggedin          bool
	previousTLSCommands []string
	readLock            bool
	gotResponse         chan struct{}
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
		readlockMutex:     &sync.Mutex{},
		log:               &logger{fromip: connection.RemoteAddr().String(), id: id},
		srcIP:             connection.RemoteAddr().String(),
		tlsProtocol:       0,
		isLoggedin:        false,
		readLock:          false,
		gotResponse:       make(chan struct{}),
	}

	// is masquerade IP not setted, set local IP of client connection
	if len(p.config.MasqueradeIP) == 0 {
		p.config.MasqueradeIP = strings.Split(connection.LocalAddr().String(), ":")[0]
	}

	return p
}

func (c *clientHandler) setClientDeadLine(t int) {
	d := time.Now().Add(time.Duration(t) * time.Second)
	if c.deadline.Unix() < d.Unix() {
		c.deadline = d
		c.conn.SetDeadline(d)
	}
}

func (c *clientHandler) handleCommands() error {
	// Check max client. If exceeded, send 530 error to client and disconnect
	if atomic.LoadInt32(c.currentConnection) >= c.config.MaxConnections {
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

		c.conn.Close()

		return err
	}

	eg := errgroup.Group{}

	err := c.connectProxy()
	if err != nil {
		return err
	}

	defer func() {
		// decrease connection count
		if c.isLoggedin {
			atomic.AddInt32(c.currentConnection, -1)
		}

		// close each connection again
		if c.conn != nil {
			c.conn.Close()
		}

		if c.proxy != nil {
			c.proxy.Close()
		}
	}()

	// run response read routine
	eg.Go(func() error { return c.getResponseFromOrigin() })

	// run command read routine
	eg.Go(func() error { return c.readClientCommands() })

	// wait until all goroutine has done
	if err = eg.Wait(); err != nil {
		if c.command == "QUIT" || err == io.EOF {
			c.log.info("client disconnected")
			err = nil
		} else if err.(net.Error).Timeout() {
			c.log.info("client disconnected by timeout")
		}
	} else {
		c.log.info("client disconnected")
	}

	return err
}

func (c *clientHandler) getResponseFromOrigin() error {
	var err error

	// close origin connection when close goroutine
	defer func() {
		// send EOF to client connection
		sendEOF(c.conn)
		c.proxy.Close()
	}()

	// サーバからのレスポンスはSuspendしない限り自動で返却される
	for {
		err = c.proxy.responseProxy()
		if err != nil {
			if err == io.EOF {
				c.log.debug("EOF from proxy connection")
				err = nil
			} else {
				c.log.err("error from origin connection: %s", err.Error())
			}

			break
		}
	}

	return err
}

func (c *clientHandler) readClientCommands() error {
	lastError := error(nil)

	// close client connection when close goroutine
	defer func() {
		// set readLock false before send EOF to proxy for avoid lock on channel write
		c.readlockMutex.Lock()
		c.readLock = false
		c.readlockMutex.Unlock()

		// send EOF to origin connection
		sendEOF(c.proxy.GetConn())
		c.conn.Close()
	}()

	for {
		c.LockClientRead()

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
					if err.Timeout() {
						c.conn.SetDeadline(time.Now().Add(time.Minute))
						r := result{
							code: 421,
							msg:  "command timeout : closing control connection",
							err:  err,
							log:  c.log,
						}
						if err := r.Response(c); err != nil {
							lastError = fmt.Errorf("response to client error: %v", err)
						}
					}
				}

				c.log.debug("error from client connection: %s", err.Error())
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

// LockClientRead wait until got response from origin after send command
func (c *clientHandler) LockClientRead() {
	c.readlockMutex.Lock()

	if c.readLock {
		c.readLock = false
		c.readlockMutex.Unlock()
	} else {
		c.readLock = true
		c.readlockMutex.Unlock()
		<-c.gotResponse
	}
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

	c.readlockMutex.Lock()
	c.readLock = true
	c.readlockMutex.Unlock()

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
		err := c.proxy.switchOrigin(c.srcIP, c.context.RemoteAddr, c.tlsProtocol, c.previousTLSCommands)
		if err != nil {
			return err
		}
	} else {
		p, err := newProxyServer(
			&proxyServerConfig{
				clientReader:   c.reader,
				clientWriter:   c.writer,
				originAddr:     c.context.RemoteAddr,
				mutex:          c.mutex,
				readlockMutex:  c.readlockMutex,
				log:            c.log,
				config:         c.config,
				readLock:       &c.readLock,
				nowGotResponse: c.gotResponse,
			})

		if err != nil {
			return err
		}
		c.proxy = p
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
	if strings.Compare(strings.ToUpper(getCommand(line)[0]), SECURE_COMMAND) == 0 {
		c.log.info("read from client: %s ********\r\n", SECURE_COMMAND)
	} else {
		c.log.info("read from client: %s", line)
	}
}
