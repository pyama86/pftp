package pftp

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type clientHandler struct {
	id          uint32               // ID of the client
	daddy       *FtpServer           // Server on which the connection was accepted
	driver      ClientHandlingDriver // Client handling driver
	conn        net.Conn             // TCP connection
	writer      *bufio.Writer        // Writer on the TCP connection
	reader      *bufio.Reader        // Reader on the TCP connection
	user        string               // Authenticated user
	path        string               // Current path
	command     string               // Command received on the connection
	param       string               // Param of the FTP command
	connectedAt time.Time            // Date of connection
	ctxRnfr     string               // Rename from
	ctxRest     int64                // Restart point
	debug       bool                 // Show debugging info on the server side
	transferTLS bool                 // Use TLS for transfer connection
	logger      log.Logger           // Client handler logging
	isAscii     bool
}

// newClientHandler initializes a client handler when someone connects
func (server *FtpServer) newClientHandler(connection net.Conn, id uint32) *clientHandler {

	p := &clientHandler{
		daddy:       server,
		conn:        connection,
		id:          id,
		writer:      bufio.NewWriter(connection),
		reader:      bufio.NewReader(connection),
		connectedAt: time.Now().UTC(),
		path:        "/",
		logger:      log.With(server.Logger, "clientId", id),
	}

	// Just respecting the existing logic here, this could be probably be dropped at some point

	return p
}

func (c *clientHandler) disconnect() {
	c.conn.Close()
}

// Path provides the current working directory of the client
func (c *clientHandler) Path() string {
	return c.path
}

// SetPath changes the current working directory
func (c *clientHandler) SetPath(path string) {
	c.path = path
}

// Debug defines if we will list all interaction
func (c *clientHandler) Debug() bool {
	return c.debug
}

// SetDebug changes the debug flag
func (c *clientHandler) SetDebug(debug bool) {
	c.debug = debug
}

// ID provides the client's ID
func (c *clientHandler) ID() uint32 {
	return c.id
}

// RemoteAddr returns the remote network address.
func (c *clientHandler) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// LocalAddr returns the local network address.
func (c *clientHandler) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *clientHandler) end() {
	c.daddy.driver.UserLeft(c)
	c.daddy.clientDeparture(c)
}

// HandleCommands reads the stream of commands
func (c *clientHandler) HandleCommands() {
	defer c.end()

	if msg, err := c.daddy.driver.WelcomeUser(c); err == nil {
		c.writeMessage(220, msg)
	} else {
		c.writeMessage(500, msg)
		return
	}

	for {
		if c.reader == nil {
			if c.debug {
				level.Debug(c.logger).Log(logKeyMsg, "Clean disconnect", logKeyAction, "ftp.disconnect", "clean", true)
			}
			return
		}

		// florent(2018-01-14): #58: IDLE timeout: Preparing the deadline before we read
		if c.daddy.settings.IdleTimeout > 0 {
			c.conn.SetDeadline(time.Now().Add(time.Duration(time.Second.Nanoseconds() * int64(c.daddy.settings.IdleTimeout))))
		}

		line, err := c.reader.ReadString('\n')

		if err != nil {
			// florent(2018-01-14): #58: IDLE timeout: Adding some code to deal with the deadline
			switch err := err.(type) {
			case net.Error:
				if err.Timeout() {
					// We have to extend the deadline now
					c.conn.SetDeadline(time.Now().Add(time.Minute))
					level.Info(c.logger).Log(logKeyMsg, "IDLE timeout", logKeyAction, "ftp.idle_timeout", "err", err)
					c.writeMessage(421, fmt.Sprintf("command timeout (%d seconds): closing control connection", c.daddy.settings.IdleTimeout))
					if err := c.writer.Flush(); err != nil {
						level.Error(c.logger).Log(logKeyMsg, "Network flush error", logKeyAction, "ftp.flush_error", "err", err)
					}
					if err := c.conn.Close(); err != nil {
						level.Error(c.logger).Log(logKeyMsg, "Network close error", logKeyAction, "ftp.close_error", "err", err)
					}
					break
				}
				level.Error(c.logger).Log(logKeyMsg, "Network error", logKeyAction, "ftp.net_error", "err", err)
			default:
				if err == io.EOF {
					if c.debug {
						level.Debug(c.logger).Log(logKeyMsg, "TCP disconnect", logKeyAction, "ftp.disconnect", "clean", false)
					}
				} else {
					level.Error(c.logger).Log(logKeyMsg, "Read error", logKeyAction, "ftp.read_error", "err", err)
				}
			}
			return
		}

		if c.debug {
			level.Debug(c.logger).Log(logKeyMsg, "FTP RECV", logKeyAction, "ftp.cmd_recv", "line", line)
		}

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

	if c.driver == nil && !cmdDesc.Open {
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
		level.Debug(c.logger).Log(logKeyMsg, "FTP SEND", logKeyAction, "ftp.cmd_send", "line", line)
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

func (c *clientHandler) handleAUTH() {
	if tlsConfig, err := c.daddy.driver.GetTLSConfig(); err == nil {
		c.writeMessage(234, "AUTH command ok. Expecting TLS Negotiation.")
		c.conn = tls.Server(c.conn, tlsConfig)
		c.reader = bufio.NewReader(c.conn)
		c.writer = bufio.NewWriter(c.conn)
	} else {
		c.writeMessage(550, fmt.Sprintf("Cannot get a TLS config: %v", err))
	}
}
