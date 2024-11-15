package pftp

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/BurntSushi/toml"
)

const (
	// PortRangeLength is const parameter for port range configuration check
	// port range must set like 100-110 so it might split 2 strings by '-'
	PortRangeLength = 2
)

type config struct {
	ListenAddr      string   `toml:"listen_addr"`
	RemoteAddr      string   `toml:"remote_addr"`
	IdleTimeout     int      `toml:"idle_timeout"`
	ProxyTimeout    int      `toml:"proxy_timeout"`
	TransferTimeout int      `toml:"transfer_timeout"`
	MaxConnections  int32    `toml:"max_connections"`
	ProxyProtocol   bool     `toml:"send_proxy_protocol"`
	WelcomeMsg      string   `toml:"welcome_message"`
	KeepaliveTime   int      `toml:"keepalive_time"`
	DataChanProxy   bool     `toml:"data_channel_proxy"`
	DataPortRange   string   `toml:"data_listen_port_range"`
	MasqueradeIP    string   `toml:"masquerade_ip"`
	TransferMode    string   `toml:"transfer_mode"`
	IgnorePassiveIP bool     `toml:"ignore_passive_ip"`
	TLS             *tlsPair `toml:"tls"`
}

// NewConfig creates a new config instance and applies the provided options.
// Validates the configuration and returns an error if validation fails.
func NewConfig(opts ...ConfigOption) (config, error) {
	cfg := config{}
	defaultConfig(&cfg)

	for _, o := range opts {
		o(&cfg)
	}

	if err := validateConfig(&cfg); err != nil {
		return config{}, err
	}

	return cfg, nil
}

func validateConfig(c *config) error {
	// validate Data listen port randg
	if err := dataPortRangeValidation(c.DataPortRange); err != nil {
		logrus.Debug(err)
		c.DataPortRange = ""
	}

	// validate Masquerade IP
	if (len(c.MasqueradeIP) > 0) && (net.ParseIP(c.MasqueradeIP)) == nil {
		return fmt.Errorf("configuration error: Masquerade IP is wrong")
	}

	// validate Transfer mode config
	c.TransferMode = strings.ToUpper(c.TransferMode)
	switch c.TransferMode {
	case "PORT", "ACTIVE":
		c.TransferMode = "PORT"
	case "PASV", "PASSIVE":
		c.TransferMode = "PASV"
	case "EPSV":
		c.TransferMode = "EPSV"
	case "CLIENT":
		break
	default:
		return fmt.Errorf("configuration error: Transfer mode config is wrong")
	}

	return nil
}

type tlsPair struct {
	Cert        string `toml:"cert"`
	Key         string `toml:"key"`
	CACert      string `toml:"ca_cert"`
	CipherSuite string `toml:"cipher_suite"`
	MinProtocol string `toml:"min_protocol"`
	MaxProtocol string `toml:"max_protocol"`
}

// NewTLSConfig creates a new tlsPair instance and applies the provided options.
// Returns the configured TLS settings.
func NewTLSConfig(opts ...TLSConfigOption) tlsPair {
	tls := tlsPair{}

	for _, o := range opts {
		o(&tls)
	}

	return tls
}

func loadConfig(path string) (*config, error) {
	var c config
	defaultConfig(&c)

	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, err
	}

	if err := validateConfig(&c); err != nil {
		return nil, err
	}

	return &c, nil
}

func defaultConfig(config *config) {
	config.ListenAddr = "127.0.0.1:2121"
	config.IdleTimeout = 900
	config.ProxyTimeout = 900
	config.TransferTimeout = 900
	config.KeepaliveTime = 900
	config.ProxyProtocol = false
	config.DataChanProxy = false
	config.DataPortRange = ""
	config.WelcomeMsg = "FTP proxy ready"
	config.TransferMode = "CLIENT"
	config.IgnorePassiveIP = false
}

func dataPortRangeValidation(r string) error {
	if len(r) == 0 {
		return nil
	}

	lastErr := fmt.Errorf("data port range config wrong. set default(random port)")
	portRange := strings.Split(r, "-")

	if len(portRange) != PortRangeLength {
		return lastErr
	}

	min, err := strconv.Atoi(strings.TrimSpace(portRange[0]))
	if err != nil {
		return lastErr
	}
	max, err := strconv.Atoi(strings.TrimSpace(portRange[1]))
	if err != nil {
		return lastErr
	}

	// check each configs
	if min <= 0 || min > 65535 || max <= 0 || max > 65535 || min > max {
		return lastErr
	}

	return nil
}

type ConfigOption func(c *config)

// WithListenAddr sets the listening address for the server.
func WithListenAddr(addr string) ConfigOption {
	return func(c *config) {
		c.ListenAddr = addr
	}
}

// WithRemoteAddr sets the remote address for the server.
func WithRemoteAddr(addr string) ConfigOption {
	return func(c *config) {
		c.RemoteAddr = addr
	}
}

// WithIdleTimeout sets the idle timeout for the server in seconds.
func WithIdleTimeout(timeout int) ConfigOption {
	return func(c *config) {
		c.IdleTimeout = timeout
	}
}

// WithProxyTimeout sets the timeout for the proxy connection in seconds.
func WithProxyTimeout(timeout int) ConfigOption {
	return func(c *config) {
		c.ProxyTimeout = timeout
	}
}

// WithTransferTimeout sets the timeout for data transfers in seconds.
func WithTransferTimeout(timeout int) ConfigOption {
	return func(c *config) {
		c.TransferTimeout = timeout
	}
}

// WithMaxConnections sets the maximum number of simultaneous connections.
func WithMaxConnections(maxConn int32) ConfigOption {
	return func(c *config) {
		c.MaxConnections = maxConn
	}
}

// WithProxyProtocol enables or disables the proxy protocol.
func WithProxyProtocol(proxyProtocol bool) ConfigOption {
	return func(c *config) {
		c.ProxyProtocol = proxyProtocol
	}
}

// WithWelcomeMessage sets the welcome message for the server.
func WithWelcomeMessage(msg string) ConfigOption {
	return func(c *config) {
		c.WelcomeMsg = msg
	}
}

// WithKeepaliveTime sets the keep-alive time for the server in seconds.
func WithKeepaliveTime(keepalive int) ConfigOption {
	return func(c *config) {
		c.KeepaliveTime = keepalive
	}
}

// WithDataChanProxy enables or disables data channel proxying.
func WithDataChanProxy(dataChanProxy bool) ConfigOption {
	return func(c *config) {
		c.DataChanProxy = dataChanProxy
	}
}

// WithDataPortRange sets the range of data ports available for the server.
func WithDataPortRange(dataPortRange string) ConfigOption {
	return func(c *config) {
		c.DataPortRange = dataPortRange
	}
}

// WithMasqueradeIP sets the IP address for masquerading connections.
func WithMasqueradeIP(masqueradeIP string) ConfigOption {
	return func(c *config) {
		c.MasqueradeIP = masqueradeIP
	}
}

// WithTransferMode sets the transfer mode for data transfers.
func WithTransferMode(transferMode string) ConfigOption {
	return func(c *config) {
		c.TransferMode = transferMode
	}
}

// WithIgnorePassiveIP enables or disables ignoring of the passive IP address.
func WithIgnorePassiveIP(ignorePassiveIP bool) ConfigOption {
	return func(c *config) {
		c.IgnorePassiveIP = ignorePassiveIP
	}
}

// WithTLSConfig sets the TLS configuration for the server.
func WithTLSConfig(tls *tlsPair) ConfigOption {
	return func(c *config) {
		c.TLS = tls
	}
}

type TLSConfigOption func(t *tlsPair)

// WithCertificate sets the certificate file for TLS configuration.
func WithCertificate(cert string) TLSConfigOption {
	return func(t *tlsPair) {
		t.Cert = cert
	}
}

// WithKey sets the key file for TLS configuration.
func WithKey(key string) TLSConfigOption {
	return func(t *tlsPair) {
		t.Key = key
	}
}

// WithCACert sets the CA certificate file for TLS configuration.
func WithCACert(ca string) TLSConfigOption {
	return func(t *tlsPair) {
		t.CACert = ca
	}
}

func WithCipherSuite(cs string) TLSConfigOption {
	return func(t *tlsPair) {
		t.CipherSuite = cs
	}
}

// WithMinProtocol sets the minimum protocol version for TLS configuration.
func WithMinProtocol(minProtocol string) TLSConfigOption {
	return func(t *tlsPair) {
		t.MinProtocol = minProtocol
	}
}

// WithMaxProtocol sets the maximum protocol version for TLS configuration.
func WithMaxProtocol(maxProtocol string) TLSConfigOption {
	return func(t *tlsPair) {
		t.MaxProtocol = maxProtocol
	}
}
