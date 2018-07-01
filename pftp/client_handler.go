package pftp

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

type handleFunc struct {
	f       func(*clientHandler) *result
	suspend bool
}

var handlers map[string]*handleFunc

func init() {
	handlers = make(map[string]*handleFunc)
	handlers["USER"] = &handleFunc{(*clientHandler).handleUSER, true}
	handlers["AUTH"] = &handleFunc{(*clientHandler).handleAUTH, true}
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
	controleProxy     *ProxyServer
	context           *Context
	currentConnection *int32
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
	}

	return p
}

func (c *clientHandler) WelcomeUser() *result {
	current := atomic.AddInt32(c.currentConnection, 1)
	if current > c.config.MaxConnections {
		return &result{
			code: 500,
			msg:  "Cannot accept any additional client",
			err:  fmt.Errorf("too many clients: %d > %d", current, c.config.MaxConnections),
		}
	}
	return &result{
		code: 220,
		msg:  "Welcome on ftpserver",
	}
}

func (c *clientHandler) end() {
	atomic.AddInt32(c.currentConnection, -1)
}
func (c *clientHandler) HandleCommands() error {
	defer c.end()
	done := make(chan struct{})
	proxyError := make(chan error)

	defer func() {
		if c.controleProxy != nil {
			c.controleProxy.Close()
			<-done
		}
	}()

	err := c.connectControlProxy()
	if err != nil {
		return err
	}

	res := c.WelcomeUser()
	if res != nil {
		if err := res.Response(c); err != nil {
			return err
		}
	}
	// サーバからのレスポンスはSuspendしない限り自動で返却される
	go func() {
		for {
			if err := c.controleProxy.DownloadProxy(); err != nil {
				if c.controleProxy.CloseOk && c.controleProxy.Switch {
					c.controleProxy.CloseOk = false
					c.controleProxy.Switch = false
					continue
				} else if !c.controleProxy.CloseOk {
					proxyError <- err
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
				c.conn.SetDeadline(time.Now().Add(time.Duration(c.config.IdleTimeout) * time.Second))
			}

			line, err := c.reader.ReadString('\n')

			logrus.Debugf("[%d]read from client: %s", c.id, line)
			if err != nil {
				switch err := err.(type) {
				case net.Error:
					if err.Timeout() {
						c.conn.SetDeadline(time.Now().Add(time.Minute))
						logrus.Infof("[%d]IDLE timeout", c.id)
						r := result{
							code: 421,
							msg:  fmt.Sprintf("command timeout (%d seconds): closing control connection", c.config.IdleTimeout),
							err:  err,
						}
						if err := r.Response(c); err != nil {
							return err
						}

						if err := c.writer.Flush(); err != nil {
							logrus.Errorf("[%d]Network flush error", c.id)
						}
						if err := c.conn.Close(); err != nil {
							logrus.Errorf("[%d]Network close error", c.id)
						}
						return errors.New("idle timeout")
					}
					return err
				default:
					return err
				}
			}

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
	if _, err := c.writer.Write([]byte(line)); err != nil {
		return err
	}
	logrus.Debugf("[%d]send to client:%s", c.id, line)
	if _, err := c.writer.Write([]byte("\r\n")); err != nil {
		return err
	}
	if err := c.writer.Flush(); err != nil {
		return err
	}
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
			c.controleProxy.Suspend()
			defer c.controleProxy.Unsuspend()
		}
		res := cmd.f(c)
		if res != nil {
			return res
		}
	} else {
		if err := c.controleProxy.SendToOrigin(line); err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Internal error: %s", err),
			}
		}
	}

	return nil
}

func (c *clientHandler) connectControlProxy() error {
	if c.controleProxy != nil {
		err := c.controleProxy.SwitchOrigin(c.context.RemoteAddr)
		if err != nil {
			return err
		}
	} else {
		p, err := NewProxyServer(c.config.ProxyTimeout, c.conn, c.context.RemoteAddr, c.id)
		if err != nil {
			return err
		}
		c.controleProxy = p
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
