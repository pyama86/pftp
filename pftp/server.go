package pftp

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"

	"github.com/BurntSushi/toml"
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
	SettingsFile  string
	settings      *Settings // General settings
	listener      net.Listener
	ClientCounter uint32
	config        OurSettings // Our settings
}

type OurSettings struct {
	Server         Settings // Server settings (shouldn't need to be filled)
	MaxConnections uint32   `toml:"max_connections"`
}

func (server *FtpServer) loadSettings() error {
	err := server.GetSettings()

	if err != nil {
		return err
	}

	if server.config.Server.Listener == nil && server.config.Server.ListenAddr == "" {
		server.settings.ListenAddr = "0.0.0.0:2121"
	}

	if server.config.Server.IdleTimeout == 0 {
		server.settings.IdleTimeout = 900
	}

	return nil
}

func (server *FtpServer) GetSettings() error {
	f, err := os.Open(server.SettingsFile)
	if err != nil {
		return err
	}
	defer f.Close()

	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	if err := toml.Unmarshal(buf, &server.config); err != nil {
		return fmt.Errorf("problem loading \"%s\": %v", server.SettingsFile, err)
	}

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
		return err
	}

	logrus.Info("Listening ftp.listening address ", server.listener.Addr())

	return err
}

// Serve accepts and process any new client coming
func (server *FtpServer) Serve() error {
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			if server.listener != nil {
				return err
			}
		}

		server.clientArrival(connection)
	}
	return nil
}

// ListenAndServe simply chains the Listen and Serve method calls
func (server *FtpServer) ListenAndServe() error {
	if err := server.Listen(); err != nil {
		return err
	}

	logrus.Info("Starting...ftp.starting")

	return server.Serve()
}

func NewFtpServer(confFile string) *FtpServer {
	return &FtpServer{
		SettingsFile: confFile,
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
	server.ClientCounter++
	id := server.ClientCounter

	c := server.newClientHandler(conn, id)
	go c.HandleCommands()

	logrus.Info("FTP Client connected ftp.connected ", "clientIp ", conn.RemoteAddr())

	return nil
}
