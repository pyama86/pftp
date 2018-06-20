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
	handlers["USER"] = &handleFunc{(*clientHandler).handleUSER, false}
	handlers["AUTH"] = &handleFunc{(*clientHandler).handleAUTH, true}
	handlers["EPSV"] = &handleFunc{(*clientHandler).handlePASV, true}
	handlers["PASV"] = &handleFunc{(*clientHandler).handlePASV, true}
	handlers["PORT"] = &handleFunc{(*clientHandler).handlePORT, true}
	handlers["MLSD"] = &handleFunc{(*clientHandler).handleLIST, true}

	// transfer files
	handlers["RETR"] = &handleFunc{(*clientHandler).handleRETR, false}
	handlers["STOR"] = &handleFunc{(*clientHandler).handleSTOR, false}
	handlers["APPE"] = &handleFunc{(*clientHandler).handleAPPE, false}
	handlers["LIST"] = &handleFunc{(*clientHandler).handleLIST, true}
}

type clientHandler struct {
	conn              net.Conn
	config            *config
	middleware        middleware
	writer            *bufio.Writer
	reader            *bufio.Reader
	line              string
	command           string
	param             string
	transfer          transferHandler
	transferTLS       bool
	controleProxy     *ProxyServer
	context           *Context
	currentConnection *int32
}

func newClientHandler(connection net.Conn, c *config, m middleware, currentConnection *int32) *clientHandler {
	p := &clientHandler{
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
	var proxyError error
	done := make(chan struct{})

	defer func() {
		if c.controleProxy != nil {
			c.controleProxy.Close()
			<-done
		}
	}()

	res := c.WelcomeUser()
	if res != nil {
		if err := res.Response(c); err != nil {
			return err
		}
	}
	go func() {
		for {
			if c.controleProxy != nil {
				if err := c.controleProxy.DownloadProxy(); err != nil {
					logrus.Errorf("Response Proxy error: %s", err)
					proxyError = err
					break
				}
			} else {
				break
			}
		}
		done <- struct{}{}
	}()

	for {
		if proxyError != nil {
			return proxyError
		}

		if c.config.IdleTimeout > 0 {
			c.conn.SetDeadline(time.Now().Add(time.Duration(c.config.IdleTimeout) * time.Second))
		}

		line, err := c.reader.ReadString('\n')
		logrus.Debug("read from client:", line)
		if err != nil {
			switch err := err.(type) {
			case net.Error:
				if err.Timeout() {
					c.conn.SetDeadline(time.Now().Add(time.Minute))
					logrus.Info("IDLE timeout")
					r := result{
						code: 421,
						msg:  fmt.Sprintf("command timeout (%d seconds): closing control connection", c.config.IdleTimeout),
						err:  err,
					}
					if err := r.Response(c); err != nil {
						return err
					}

					if err := c.writer.Flush(); err != nil {
						logrus.Error("Network flush error")
					}
					if err := c.conn.Close(); err != nil {
						logrus.Error("Network close error")
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

func (c *clientHandler) writeLine(line string) error {
	if _, err := c.writer.Write([]byte(line)); err != nil {
		return err
	}
	logrus.Debug("send to client:", line)
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
	cmd := handlers[c.command]
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

	if cmd != nil {
		if c.controleProxy != nil {
			if cmd.suspend {
				c.controleProxy.Suspend()
			}
		}
		res := cmd.f(c)
		if res != nil {
			return res
		}
	} else {
		if c.controleProxy != nil {
			if err := c.controleProxy.SendToOrigin(line); err != nil {
				return &result{
					code: 500,
					msg:  fmt.Sprintf("Internal error: %s", err),
				}
			}
		}
	}

	if c.controleProxy != nil {
		c.controleProxy.Unsuspend()
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
