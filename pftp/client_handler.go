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
	id          uint32               // ID of the client
	daddy       *FtpServer           // Server on which the connection was accepted
	driver      ClientHandlingDriver // Client handling driver
	conn        net.Conn             // TCP connection
	writer      *bufio.Writer        // Writer on the TCP connection
	reader      *bufio.Reader        // Reader on the TCP connection
	user        string               // Authenticated user
	command     string               // Command received on the connection
	param       string               // Param of the FTP command
	connectedAt time.Time            // Date of connection
	ctxRnfr     string               // Rename from
	ctxRest     int64                // Restart point
	debug       bool                 // Show debugging info on the server side
	transferTLS bool                 // Use TLS for transfer connection
	proxy       *ProxyServer
}

func (server *FtpServer) newClientHandler(connection net.Conn, id uint32) *clientHandler {
	p := &clientHandler{
		daddy:       server,
		conn:        connection,
		id:          id,
		writer:      bufio.NewWriter(connection),
		reader:      bufio.NewReader(connection),
		connectedAt: time.Now().UTC(),
	}

	return p
}

func (c *clientHandler) disconnect() {
	c.conn.Close()
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
	c.daddy.ClientCounter--
}

// WelcomeUser is called to send the very first welcome message
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
			logrus.Debug("Clean disconnect ftp.disconnect")
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
					logrus.Info("IDLE timeout ftp.idle_timeout")
					c.writeMessage(421, fmt.Sprintf("command timeout (%d seconds): closing control connection", c.daddy.settings.IdleTimeout))
					if err := c.writer.Flush(); err != nil {
						logrus.Error("Network flush error ftp.flush_error")
					}
					if err := c.conn.Close(); err != nil {
						logrus.Error("Network close error ftp.close_error")
					}
					break
				}
				logrus.Error("Network error ftp.net_error")
			default:
				if err == io.EOF {
					if c.debug {
						logrus.Debug("TCP disconnect ftp.disconnect")
					}
				} else {
					logrus.Error("Read error ftp.read_error")
				}
			}
			return
		}

		c.handleCommand(line)

	}
}

func (c *clientHandler) handleCommand(line string) {
	command, param := parseLine(line)
	c.command = strings.ToUpper(command)
	c.param = param

	if c.command == "USER" || c.command == "AUTH" {
		cmdDesc := commandsMap[c.command]

		defer func() {
			if r := recover(); r != nil {
				c.writeMessage(500, fmt.Sprintf("Internal error: %s", r))
			}
		}()

		cmdDesc.Fn(c)
	}

	if c.proxy != nil {
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

func (c *clientHandler) handleUSER() {
	c.user = c.param
	p, err := NewProxyServer(c.param, c.conn, "localhost:2321")
	if err != nil {
		c.writeMessage(530, "I can't deal with you (proxy error)")
	}

	c.proxy = p
}

func (c *clientHandler) handleAUTH() {
	//	if tlsConfig, err := c.daddy.driver.GetTLSConfig(); err == nil {
	//		c.writeMessage(234, "AUTH command ok. Expecting TLS Negotiation.")
	//		c.conn = tls.Server(c.conn, tlsConfig)
	//		c.reader = bufio.NewReader(c.conn)
	//		c.writer = bufio.NewWriter(c.conn)
	//	} else {
	//c.writeMessage(550, fmt.Sprintf("Cannot get a TLS config: %v", err))
	//	}
	c.writeMessage(550, fmt.Sprintf("Cannot get a TLS config: %v", "hoge"))
}
