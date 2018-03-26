package pftp

import (
	"crypto/tls"
	"io"
	"net"
)

type MainDriver interface {
	GetSettings() (*Settings, error)
	WelcomeUser(cc ClientContext) (string, error)
	UserLeft(cc ClientContext)
	AuthUser(cc ClientContext, user, pass string) (ClientHandlingDriver, error)
	GetTLSConfig() (*tls.Config, error)
}

type ClientHandlingDriver interface {
}

type ClientContext interface {
	ID() uint32
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
}

// FileStream is a read or write closeable stream
type FileStream interface {
	io.Writer
	io.Reader
	io.Closer
	io.Seeker
}

// PortRange is a range of ports
type PortRange struct {
	Start int // Range start
	End   int // Range end
}

// PublicIPResolver takes a ClientContext for a connection and returns the public IP
// to use in the response to the PASV command, or an error if a public IP cannot be determined.
type PublicIPResolver func(ClientContext) (string, error)

// Settings defines all the server settings
type Settings struct {
	Listener                  net.Listener     // Allow providing an already initialized listener. Mutually exclusive with ListenAddr
	ListenAddr                string           // Listening address
	PublicHost                string           // Public IP to expose (only an IP address is accepted at this stage)
	PublicIPResolver          PublicIPResolver // Optional function that can perform a public ip lookup for the given CientContext.
	DataPortRange             *PortRange       // Port Range for data connections. Random one will be used if not specified
	DisableMLSD               bool             // Disable MLSD support
	DisableMLST               bool             // Disable MLST support
	NonStandardActiveDataPort bool             // Allow to use a non-standard active data port
	IdleTimeout               int              // Maximum inactivity time before disconnecting (#58)
}
