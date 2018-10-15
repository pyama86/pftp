package pftp

import (
	"bufio"
	"errors"
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
	handlers["RETR"] = &handleFunc{(*clientHandler).handleTransfer, false}
	handlers["STOR"] = &handleFunc{(*clientHandler).handleTransfer, false}
}

type clientHandler struct {
	id                int
	conn              net.Conn
	config            *config
	middleware        middleware
	writer            *bufio.Writer
	reader            *bufio.Reader
	line              string
	command           string
	param             string
	proxy             *proxyServer
	context           *Context
	currentConnection *int32
	mutex             *sync.Mutex
	log               *logger
	deadline          time.Time
	sourceIP          string
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
		sourceIP:          connection.RemoteAddr().String(),
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
	defer c.end()
	done := make(chan struct{})
	proxyError := make(chan error)

	defer func() {
		close(proxyError)
		if c.proxy != nil {
			c.proxy.Close()
			<-done
		}
	}()

	err := c.connectProxy()
	if err != nil {
		return err
	}

	// サーバからのレスポンスはSuspendしない限り自動で返却される
	go func() {
		for {
			err := c.proxy.responseProxy()
			if err != nil {
				if err != io.EOF {
					safeSetChanel(proxyError, err)
				}
				break
			}
		}
		done <- struct{}{}
	}()

	for {
		select {
		case e := <-proxyError:
			return e
		default:
			if c.config.IdleTimeout > 0 {
				c.setClientDeadLine(c.config.IdleTimeout)
			}
			line, err := c.reader.ReadString('\n')
			if err != nil {
				// client disconnect
				if err == io.EOF {
					if err := c.conn.Close(); err != nil {
						c.log.err("Network close error")
					}

					return nil
				}
				switch err := err.(type) {
				case net.Error:
					if err.Timeout() {
						c.conn.SetDeadline(time.Now().Add(time.Minute))
						c.log.info("IDLE timeout")
						r := result{
							code: 421,
							msg:  "command timeout : closing control connection",
							err:  err,
							log:  c.log,
						}
						if err := r.Response(c); err != nil {
							return err
						}

						if err := c.writer.Flush(); err != nil {
							c.log.err("Network flush error")
						}

						if err := c.conn.Close(); err != nil {
							c.log.err("Network close error")
						}
						return errors.New("idle timeout")
					}
					return err
				default:
					return err
				}
			}

			c.log.debug("read from client: %s", line)
			commandResponse := c.handleCommand(line)
			if commandResponse != nil {
				if err := commandResponse.Response(c); err != nil {
					return err
				}
			}
		}
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
		err := c.proxy.switchOrigin(c.sourceIP, c.context.RemoteAddr)
		if err != nil {
			return err
		}
	} else {
		p, err := newProxyServer(
			&proxyServerConfig{
				timeout:       c.config.ProxyTimeout,
				clientReader:  c.reader,
				clientWriter:  c.writer,
				originAddr:    c.context.RemoteAddr,
				mutex:         c.mutex,
				log:           c.log,
				proxyProtocol: c.config.ProxyProtocol,
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

func (c *clientHandler) getSourceIPFromProxyHeader(line string) error {
	params := strings.SplitN(strings.Trim(line, "\r\n"), " ", 6)
	if len(params) != 6 {
		return errors.New("wrong proxy header parameters")
	}

	sourceIP := params[2]
	if net.ParseIP(sourceIP) == nil {
		return errors.New("wrong ip address")
	}

	sourcePort := params[4]

	c.sourceIP = sourceIP + ":" + sourcePort

	return nil
}
