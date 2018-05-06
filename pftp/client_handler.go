package pftp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
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
	conn          net.Conn
	config        *config
	middleware    middleware
	writer        *bufio.Writer
	reader        *bufio.Reader
	line          string
	command       string
	param         string
	transfer      transferHandler
	transferTLS   bool
	controleProxy *ProxyServer
	context       *Context
}

func (ftp *FtpServer) newClientHandler(connection net.Conn, c *config, m middleware) *clientHandler {
	p := &clientHandler{
		conn:       connection,
		config:     c,
		middleware: m,
		writer:     bufio.NewWriter(connection),
		reader:     bufio.NewReader(connection),
		context:    newContext(c),
	}

	return p
}

func (c *clientHandler) WelcomeUser() *result {
	return &result{
		code: 220,
		msg:  "Welcome on ftpserver",
	}
}

func (c *clientHandler) HandleCommands() {
	defer func() {
		if c.controleProxy != nil {
			c.controleProxy.Close()
		}
	}()

	res := c.WelcomeUser()
	if res != nil {
		res.Response(c)
	}

	go func() {
		for {
			if c.controleProxy != nil {
				if err := c.controleProxy.Start(false); err != nil {
					logrus.Errorf("Response Proxy error: %s", err)
					break
				}
			}
		}
	}()

	for {
		if c.config.IdleTimeout > 0 {
			c.conn.SetDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(c.config.IdleTimeout))))
		}

		line, err := c.reader.ReadString('\n')
		logrus.Debug("read from client:", line)
		if err != nil {
			switch err := err.(type) {
			case net.Error:
				if err.Timeout() {
					c.conn.SetDeadline(time.Now().Add(time.Minute))
					logrus.Info("IDLE timeout")
					c.writeMessage(421, fmt.Sprintf("command timeout (%d seconds): closing control connection", c.config.IdleTimeout))
					if err := c.writer.Flush(); err != nil {
						logrus.Error("Network flush error")
					}
					if err := c.conn.Close(); err != nil {
						logrus.Error("Network close error")
					}
					break
				}
				logrus.Error("Network error ftp.net_error")
			default:
				if err == io.EOF {
					logrus.Debug("TCP disconnect")
				} else {
					logrus.Error("Read error")
				}
			}
			return
		}
		c.handleCommand(line)
	}
}

func (c *clientHandler) writeLine(line string) {
	c.writer.Write([]byte(line))
	logrus.Debug("send to client:", line)
	c.writer.Write([]byte("\r\n"))
	c.writer.Flush()
}

func (c *clientHandler) writeMessage(code int, message string) {
	line := fmt.Sprintf("%d %s", code, message)
	c.writeLine(line)
}

func (c *clientHandler) handleCommand(line string) {
	c.parseLine(line)
	cmd := handlers[c.command]
	defer func() {
		if r := recover(); r != nil {
			c.writeMessage(500, fmt.Sprintf("Internal error: %s", r))
		}
	}()

	if c.middleware[c.command] != nil {
		if err := c.middleware[c.command](c.context, c.param); err != nil {
			c.writeMessage(500, fmt.Sprintf("Internal error: %s", err.Error))
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
			res.Response(c)
		}
	} else {
		if c.controleProxy != nil {
			if err := c.controleProxy.SendToOrigin(line); err != nil {
				c.writeMessage(500, fmt.Sprintf("Internal error: %s", err.Error()))
			}
		}
	}

	if c.controleProxy != nil {
		c.controleProxy.Unsuspend()
	}
}

func (c *clientHandler) parseLine(line string) {
	params := strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)
	c.line = line
	c.command = strings.ToUpper(params[0])
	if len(params) > 1 {
		c.param = params[1]
	}
}
