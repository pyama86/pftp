package pftp

import (
	"net"

	"github.com/sirupsen/logrus"
)

type CommandDescription struct {
	Open bool                 // Open to clients without auth
	Fn   func(*clientHandler) // Function to handle it
}

var commandsMap map[string]*CommandDescription

func init() {
	commandsMap = make(map[string]*CommandDescription)
	commandsMap["USER"] = &CommandDescription{Fn: (*clientHandler).handleUSER, Open: true}
}

// Settings defines all the server settings
type Settings struct {
	Listener                  net.Listener // Allow providing an already initialized listener. Mutually exclusive with ListenAddr
	ListenAddr                string       // Listening address
	PublicHost                string       // Public IP to expose (only an IP address is accepted at this stage)
	DisableMLSD               bool         // Disable MLSD support
	DisableMLST               bool         // Disable MLST support
	NonStandardActiveDataPort bool         // Allow to use a non-standard active data port
	IdleTimeout               int          // Maximum inactivity time before disconnecting (#58)
}
type FtpServer struct {
	settings      *Settings
	listener      net.Listener
	clientCounter uint32
}

func (server *FtpServer) Listen() error {
	var err error
	server.listener, err = net.Listen("tcp", "127.0.0.1:2121")

	if err != nil {
		logrus.Error("Cannot listen", "err", err)
		return err
	}

	logrus.Info("Listening...", "ftp.listening", "address", server.listener.Addr())

	return err
}

func (server *FtpServer) Serve() {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			if server.listener != nil {
				logrus.Error("Accept error", "err", err)
			}
			break
		}

		server.clientArrival(connection)
	}
}

func (server *FtpServer) ListenAndServe() error {
	if err := server.Listen(); err != nil {
		return err
	}

	logrus.Info("Starting...", "ftp.starting")
	server.Serve()
	return nil
}

func NewFtpServer() *FtpServer {
	return &FtpServer{
		settings: &Settings{},
	}
}

func (server *FtpServer) Addr() string {
	if server.listener != nil {
		return server.listener.Addr().String()
	}
	return ""
}

func (server *FtpServer) Stop() {
	if server.listener != nil {
		server.listener.Close()
	}
}

func (server *FtpServer) clientArrival(conn net.Conn) error {
	server.clientCounter++
	id := server.clientCounter

	c := server.newClientHandler(conn, id)
	go c.HandleCommands()

	logrus.Info("FTP Client connected ftp.connected clientIp ", conn.RemoteAddr())

	return nil
}

func (server *FtpServer) clientDeparture(c *clientHandler) {
	logrus.Info("FTP Client disconnected", "ftp.disconnected", "clientIp", c.conn.RemoteAddr())
}
