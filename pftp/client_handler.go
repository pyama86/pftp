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

var handlers map[string]func(*clientHandler) *result

func init() {
	handlers = make(map[string]func(*clientHandler) *result)
	handlers["USER"] = (*clientHandler).handleUSER
	handlers["AUTH"] = (*clientHandler).handleAUTH
	handlers["EPSV"] = (*clientHandler).handlePASV
	handlers["PASV"] = (*clientHandler).handlePASV
	handlers["PORT"] = (*clientHandler).handlePORT
	handlers["LIST"] = (*clientHandler).handleLIST
	handlers["MLSD"] = (*clientHandler).handleLIST
	handlers["FEAT"] = (*clientHandler).handleFEAT

	// transfer files
	handlers["RETR"] = (*clientHandler).handleRETR
	handlers["STOR"] = (*clientHandler).handleSTOR
	handlers["APPE"] = (*clientHandler).handleAPPE
}

type clientHandler struct {
	server         *FtpServer
	conn          net.Conn
	writer        *bufio.Writer
	reader        *bufio.Reader
	line          string
	command       string
	param         string
	transfer      transferHandler
	transferTLS   bool
	controleProxy *ProxyServer
}

func (server *FtpServer) newClientHandler(connection net.Conn) *clientHandler {
	p := &clientHandler{
		server:  server,
		conn:   connection,
		writer: bufio.NewWriter(connection),
		reader: bufio.NewReader(connection),
	}

	return p
}

func (c *clientHandler) disconnect() {
	c.conn.Close()
}

func (c *clientHandler) end() {
	c.server.ClientCounter--
}

func (c *clientHandler) WelcomeUser() *result {
	if c.server.ClientCounter > c.server.config.MaxConnections {
		return &result{
			code: 500,
			err:  fmt.Errorf("Cannot accept any additional client"),
		}
	}

	return &result{
		code: 220,
		msg:  "Welcome on ftpserver",
	}
}

func (c *clientHandler) HandleCommands() {
	defer c.end()
	res := c.WelcomeUser()
	if res != nil {
		res.Response(c)
	}
	for {
		if c.reader == nil {
			logrus.Debug("Clean disconnect")
			return
		}

		if c.server.config.IdleTimeout > 0 {
			c.conn.SetDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(c.server.config.IdleTimeout))))
		}

		line, err := c.reader.ReadString('\n')
		logrus.Debug("read from client:", line)
		if err != nil {
			switch err := err.(type) {
			case net.Error:
				if err.Timeout() {
					c.conn.SetDeadline(time.Now().Add(time.Minute))
					logrus.Info("IDLE timeout")
					c.writeMessage(421, fmt.Sprintf("command timeout (%d seconds): closing control connection", c.server.config.IdleTimeout))
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

	if cmd != nil {
		res := cmd(c)
		if res != nil {
			res.Response(c)
		}
	} else {
		if err := c.controleProxy.SendToOriginWithProxy(line); err != nil {
			c.writeMessage(500, fmt.Sprintf("Internal error: %s", err.Error()))
		}
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
