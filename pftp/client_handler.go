package pftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type clientHandler struct {
	id          uint32        // ID of the client
	daddy       *FtpServer    // Server on which the connection was accepted
	conn        net.Conn      // TCP connection
	writer      *bufio.Writer // Writer on the TCP connection
	reader      *bufio.Reader // Reader on the TCP connection
	connectedAt time.Time     // Date of connection
	ctxRnfr     string        // Rename from
	ctxRest     int64         // Restart point
	transferTLS bool          // Use TLS for transfer connection
	proxy       *ProxyServer
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

func (c *clientHandler) handleCommand(line string) {
	command, param := parseLine(line)

	switch command {
	case "USER":
		c.handleUSER(param)
	case "AUTH":
		c.handleAUTH()
	}
	defer func() {
		if r := recover(); r != nil {
			c.writeMessage(500, fmt.Sprintf("Internal error: %s", r))
		}
	}()

	if c.proxy != nil && command != "AUTH" {
		c.proxy.SendLine(line)
	}
}

func (c *clientHandler) writeLine(line string) {
	c.writer.Write([]byte(line))
	c.writer.Write([]byte("\r\n"))
	c.writer.Flush()
}

func (c *clientHandler) writeMessage(code int, message string) {
	c.writeLine(fmt.Sprintf("%d %s", code, message))
}

func parseLine(line string) (string, string) {
	params := strings.SplitN(strings.Trim(line, "\r\n"), " ", 2)
	if len(params) == 1 {
		return params[0], ""
	}
	return params[0], params[1]
}

func (c *clientHandler) handleUSER(user string) {
	p, err := NewProxyServer(user, c.conn, "localhost:2321")
	if err != nil {
		c.writeMessage(530, "I can't deal with you (proxy error)")
		return
	}

	c.proxy = p
}

func (c *clientHandler) handleAUTH() {
	if c.daddy.config.TLSConfig != nil {
		c.writeMessage(234, "AUTH command ok. Expecting TLS Negotiation.")
		c.conn = tls.Server(c.conn, c.daddy.config.TLSConfig)
		c.reader = bufio.NewReader(c.conn)
		c.writer = bufio.NewWriter(c.conn)
	} else {
		c.writeMessage(550, fmt.Sprint("Cannot get a TLS config"))
	}
}
