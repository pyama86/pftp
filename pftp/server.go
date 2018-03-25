package pftp

import (
	"fmt"
	"net"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/sirupsen/logrus"
)

const (
	// logKeyMsg is the human-readable part of the log
	logKeyMsg = "msg"
	// logKeyAction is the machine-readable part of the log
	logKeyAction = "action"
)

// CommandDescription defines which function should be used and if it should be open to anyone or only logged in users
type CommandDescription struct {
	Open bool                 // Open to clients without auth
	Fn   func(*clientHandler) // Function to handle it
}

var commandsMap map[string]*CommandDescription

func init() {
	commandsMap = make(map[string]*CommandDescription)
	commandsMap["USER"] = &CommandDescription{Fn: (*clientHandler).handleUSER, Open: true}
	commandsMap["AUTH"] = &CommandDescription{Fn: (*clientHandler).handleAUTH, Open: true}

}

type FtpServer struct {
	Logger        log.Logger // Go-Kit logger
	settings      *Settings  // General settings
	listener      net.Listener
	clientCounter uint32     // Clients counter
	driver        MainDriver // Driver to handle the client authentication and the file access driver selection
}

func (server *FtpServer) loadSettings() error {
	s, err := server.driver.GetSettings()

	if err != nil {
		return err
	}

	if s.Listener == nil && s.ListenAddr == "" {
		s.ListenAddr = "0.0.0.0:2121"
	}

	// florent(2018-01-14): #58: IDLE timeout: Default idle timeout will be set at 900 seconds
	if s.IdleTimeout == 0 {
		s.IdleTimeout = 900
	}

	server.settings = s

	return nil
}

// Listen starts the listening
// It's not a blocking call
func (server *FtpServer) Listen() error {
	err := server.loadSettings()

	if err != nil {
		return fmt.Errorf("could not load settings: %v", err)
	}

	server.listener, err = net.Listen("tcp", server.settings.ListenAddr)

	if err != nil {
		level.Error(server.Logger).Log(logKeyMsg, "Cannot listen", "err", err)
		return err
	}

	logrus.Info("Listening ftp.listening address ", server.listener.Addr())

	return err
}

// Serve accepts and process any new client coming
func (server *FtpServer) Serve() {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			if server.listener != nil {
				level.Error(server.Logger).Log(logKeyMsg, "Accept error", "err", err)
			}
			break
		}

		server.clientArrival(connection)
	}
}

// ListenAndServe simply chains the Listen and Serve method calls
func (server *FtpServer) ListenAndServe() error {
	if err := server.Listen(); err != nil {
		return err
	}

	logrus.Info("Starting...ftp.starting")

	server.Serve()

	// Note: At this precise time, the clients are still connected. We are just not accepting clients anymore.

	return nil
}

// NewFtpServer creates a new FtpServer instance
func NewFtpServer(driver MainDriver) *FtpServer {
	return &FtpServer{
		driver: driver,
		Logger: log.NewNopLogger(),
	}
}

// Addr shows the listening address
func (server *FtpServer) Addr() string {
	if server.listener != nil {
		return server.listener.Addr().String()
	}
	return ""
}

// Stop closes the listener
func (server *FtpServer) Stop() {
	if server.listener != nil {
		server.listener.Close()
	}
}

// When a client connects, the server could refuse the connection
func (server *FtpServer) clientArrival(conn net.Conn) error {
	server.clientCounter++
	id := server.clientCounter

	c := server.newClientHandler(conn, id)
	go c.HandleCommands()

	logrus.Info("FTP Client connected ftp.connected ", "clientIp ", conn.RemoteAddr())

	return nil
}

// clientDeparture
func (server *FtpServer) clientDeparture(c *clientHandler) {
	logrus.Info("FTP Client disconnected ftp.disconnected ", "clientIp ", c.conn.RemoteAddr())
}
