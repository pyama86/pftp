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

var commandsMap map[string]*CommandDescription

type CommandDescription struct {
	Open bool                 // Open to clients without auth
	Fn   func(*clientHandler) // Function to handle it
}

func init() {
	commandsMap = make(map[string]*CommandDescription)
	commandsMap["USER"] = &CommandDescription{Fn: (*clientHandler).handleUSER}
	commandsMap["AUTH"] = &CommandDescription{Fn: (*clientHandler).handleAUTH}
	commandsMap["EPSV"] = &CommandDescription{Fn: (*clientHandler).handlePASV}
	commandsMap["PASV"] = &CommandDescription{Fn: (*clientHandler).handlePASV}
	commandsMap["LIST"] = &CommandDescription{Fn: (*clientHandler).handleLIST}
	commandsMap["MLSD"] = &CommandDescription{Fn: (*clientHandler).handleLIST}
	commandsMap["FEAT"] = &CommandDescription{Fn: (*clientHandler).handleFEAT}

	// transfer files
	commandsMap["RETR"] = &CommandDescription{Fn: (*clientHandler).handleRETR}
	commandsMap["STOR"] = &CommandDescription{Fn: (*clientHandler).handleSTOR}
	commandsMap["APPE"] = &CommandDescription{Fn: (*clientHandler).handleAPPE}
}

type clientHandler struct {
	daddy         *FtpServer
	conn          net.Conn
	writer        *bufio.Writer
	reader        *bufio.Reader
	connectedAt   time.Time
	line          string
	command       string
	param         string
	transfer      transferHandler
	controlProxy  *ProxyServer
	transferProxy *ProxyServer
}

func (server *FtpServer) newClientHandler(connection net.Conn, id uint32) *clientHandler {
	p := &clientHandler{
		daddy:       server,
		conn:        connection,
		writer:      bufio.NewWriter(connection),
		reader:      bufio.NewReader(connection),
		connectedAt: time.Now().UTC(),
	}

	return p
}

func (c *clientHandler) disconnect() {
	c.conn.Close()
}

func (c *clientHandler) end() {
	c.daddy.ClientCounter--
}

func (c *clientHandler) WelcomeUser() (string, error) {
	if c.daddy.ClientCounter > c.daddy.config.MaxConnections {
		return "Cannot accept any additional client", fmt.Errorf("too many clients: %d > % d", c.daddy.ClientCounter, c.daddy.config.MaxConnections)
	}

	return fmt.Sprint("Welcome on ftpserver"), nil
}

func (c *clientHandler) HandleCommands() {
	defer c.end()
	if msg, err := c.WelcomeUser(); err == nil {
		c.writeMessage(220, msg)
	} else {
		c.writeMessage(500, msg)
		return
	}

	for {
		if c.reader == nil {
			logrus.Debug("Clean disconnect")
			return
		}

		if c.daddy.config.IdleTimeout > 0 {
			c.conn.SetDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(c.daddy.config.IdleTimeout))))
		}

		line, err := c.reader.ReadString('\n')
		logrus.Debug("read from client:", line)
		if err != nil {
			switch err := err.(type) {
			case net.Error:
				if err.Timeout() {
					c.conn.SetDeadline(time.Now().Add(time.Minute))
					logrus.Info("IDLE timeout")
					c.writeMessage(421, fmt.Sprintf("command timeout (%d seconds): closing control connection", c.daddy.config.IdleTimeout))
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
	c.writer.Write([]byte("\r\n"))
	c.writer.Flush()
}

func (c *clientHandler) writeMessage(code int, message string) {
	line := fmt.Sprintf("%d %s", code, message)
	logrus.Debug("send to client:", line)
	c.writeLine(line)
}

func parseLine(line string) (string, string) {
	params := strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)
	if len(params) == 1 {
		return params[0], ""
	}
	return params[0], params[1]
}

func (c *clientHandler) handleCommand(line string) {
	command, param := parseLine(line)
	c.command = strings.ToUpper(command)
	c.param = param
	c.line = line

	cmdDesc := commandsMap[c.command]
	defer func() {
		if r := recover(); r != nil {
			c.writeMessage(500, fmt.Sprintf("Internal error: %s", r))
		}
	}()

	if cmdDesc != nil {
		cmdDesc.Fn(c)
	} else {
		c.controlProxy.SendToOriginWithProxy(line)
	}
}
