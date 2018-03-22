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

type clientHandler struct {
	id          uint32        // ID of the client
	daddy       *FtpServer    // Server on which the connection was accepted
	conn        net.Conn      // TCP connection
	writer      *bufio.Writer // Writer on the TCP connection
	reader      *bufio.Reader // Reader on the TCP connection
	user        string        // Authenticated user
	path        string        // Current path
	command     string        // Command received on the connection
	param       string        // Param of the FTP command
	connectedAt time.Time     // Date of connection
	ctxRnfr     string        // Rename from
	ctxRest     int64         // Restart point
	debug       bool          // Show debugging info on the server side
	transferTLS bool          // Use TLS for transfer connection
}

func (server *FtpServer) newClientHandler(connection net.Conn, id uint32) *clientHandler {
	p := &clientHandler{
		daddy:       server,
		conn:        connection,
		id:          id,
		writer:      bufio.NewWriter(connection),
		reader:      bufio.NewReader(connection),
		connectedAt: time.Now().UTC(),
		path:        "/",
	}

	return p
}

func (c *clientHandler) disconnect() {
	c.conn.Close()
}

func (c *clientHandler) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *clientHandler) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *clientHandler) end() {
}

func (c *clientHandler) WelcomeUser() (string, error) {
	//	nbClients := atomic.AddInt32(&c.id, 1)
	//	if nbClients > driver.config.MaxConnections {
	//		return "Cannot accept any additional client", fmt.Errorf("too many clients: %d > % d", c.ID, driver.config.MaxConnections)
	//	}

	// This will remain the official name for now
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
			logrus.Debug("Clean disconnect ftp.disconnect clean")
			return
		}

		if c.daddy.settings.IdleTimeout > 0 {
			c.conn.SetDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(c.daddy.settings.IdleTimeout))))
		}

		line, err := c.reader.ReadString('\n')

		if err != nil {
			switch err := err.(type) {
			case net.Error:
				if err.Timeout() {
					c.conn.SetDeadline(time.Now().Add(time.Minute))
					logrus.Info("IDLE timeout", "ftp.idle_timeout", "err", err)
					c.writeMessage(421, fmt.Sprintf("command timeout (%d seconds): closing control connection", c.daddy.settings.IdleTimeout))
					if err := c.writer.Flush(); err != nil {
						logrus.Error("Network flush error", "ftp.flush_error", "err", err)
					}
					if err := c.conn.Close(); err != nil {
						logrus.Error("Network close error", "ftp.close_error", "err", err)
					}
					break
				}
				logrus.Error("Network error", "ftp.net_error", "err", err)
			default:
				if err == io.EOF {
					if c.debug {
						logrus.Debug("TCP disconnect", "ftp.disconnect", "clean", false)
					}
				} else {
					logrus.Error("Read error", "ftp.read_error", "err", err)
				}
			}
			return
		}

		logrus.Debug("FTP RECV ftp.cmd_recv line ", line)

		c.handleCommand(line)
	}
}

// handleCommand takes care of executing the received line
func (c *clientHandler) handleCommand(line string) {
	command, param := parseLine(line)
	c.command = strings.ToUpper(command)
	c.param = param

	cmdDesc := commandsMap[c.command]
	if cmdDesc == nil {
		c.writeMessage(500, "Unknown command")
		return
	}

	if !cmdDesc.Open {
		c.writeMessage(530, "Please login with USER and PASS")
		return
	}

	// Let's prepare to recover in case there's a command error
	defer func() {
		if r := recover(); r != nil {
			c.writeMessage(500, fmt.Sprintf("Internal error: %s", r))
		}
	}()
	cmdDesc.Fn(c)
}

func (c *clientHandler) writeLine(line string) {
	if c.debug {
		logrus.Debug("FTP SEND", "ftp.cmd_send", "line", line)
	}
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

func (c *clientHandler) handleUSER() {
	c.user = c.param
	c.writeMessage(331, "OK")
}

// Handle the "PASS" command
func (c *clientHandler) handlePASS() {
	//	var err error
	//	if c.driver, err = c.daddy.driver.AuthUser(c, c.user, c.param); err == nil {
	//		c.writeMessage(230, "Password ok, continue")
	//	} else if err != nil {
	//		c.writeMessage(530, fmt.Sprintf("Authentication problem: %v", err))
	//		c.disconnect()
	//	} else {
	//		c.writeMessage(530, "I can't deal with you (nil driver)")
	//		c.disconnect()
	//	}
}
