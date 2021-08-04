package pftp

import (
	"crypto/tls"
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
	ListenAddr      string      `toml:"listen_addr"`
	RemoteAddr      string      `toml:"remote_addr"`
	IdleTimeout     int         `toml:"idle_timeout"`
	ProxyTimeout    int         `toml:"proxy_timeout"`
	TransferTimeout int         `toml:"transfer_timeout"`
	MaxConnections  int32       `toml:"max_connections"`
	ProxyProtocol   bool        `toml:"send_proxy_protocol"`
	WelcomeMsg      string      `toml:"welcome_message"`
	KeepaliveTime   int         `toml:"keepalive_time"`
	DataChanProxy   bool        `toml:"data_channel_proxy"`
	DataPortRange   string      `toml:"data_listen_port_range"`
	MasqueradeIP    string      `toml:"masquerade_ip"`
	TransferMode    string      `toml:"transfer_mode"`
	IgnorePassiveIP bool        `toml:"ignore_passive_ip"`
	TLS             *tlsPair    `toml:"tls"`
	TLSConfig       *tls.Config `toml:"-"`
}

type tlsPair struct {
	Cert        string `toml:"cert"`
	Key         string `toml:"key"`
	MinProtocol string `toml:"min_protocol"`
	MaxProtocol string `toml:"max_protocol"`
}

// TLS version codes
const (
	SSLv3  = 0x0300
	TLSv1  = 0x0301
	TLSv11 = 0x0302
	TLSv12 = 0x0303
)

func getTLSProtocol(protocol string) uint16 {
	switch protocol {
	case "TLSv1":
		return TLSv1
	case "TLSv1.1":
		return TLSv11
	case "TLSv1.2":
		return TLSv12
	default:
		return TLSv1 // the default TLS protocol is TLSv1.0
	}
}

func loadConfig(path string) (*config, error) {
	var c config
	defaultConfig(&c)

	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, err
	}

	if c.TLS != nil {
		if cert, err := tls.LoadX509KeyPair(c.TLS.Cert, c.TLS.Key); err == nil {
			c.TLSConfig = &tls.Config{
				NextProtos:   []string{"ftp"},
				Certificates: []tls.Certificate{cert},
				MinVersion:   getTLSProtocol(c.TLS.MinProtocol),
				MaxVersion:   getTLSProtocol(c.TLS.MaxProtocol),
			}
		} else {
			return nil, fmt.Errorf("configuration error: %s", err.Error())
		}
	}

	// validate Data listen port randg
	if err := dataPortRangeValidation(c.DataPortRange); err != nil {
		logrus.Debug(err)
		c.DataPortRange = ""
	}

	// validate Masquerade IP
	if (len(c.MasqueradeIP) > 0) && (net.ParseIP(c.MasqueradeIP)) == nil {
		return nil, fmt.Errorf("configuration error: Masquerade IP is wrong")
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
		return nil, fmt.Errorf("configuration error: Transfer mode config is wrong")
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
