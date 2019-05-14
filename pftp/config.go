package pftp

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/BurntSushi/toml"
)

type portRange struct {
	Start int
	End   int
}

type config struct {
	ListenAddr      string      `toml:"listen_addr"`
	RemoteAddr      string      `toml:"remote_addr"`
	IdleTimeout     int         `toml:"idle_timeout"`
	ProxyTimeout    int         `toml:"proxy_timeout"`
	TransferTimeout int         `toml:"transfer_timeout"`
	MaxConnections  int32       `toml:"max_connections"`
	ProxyProtocol   bool        `toml:"proxy_protocol"`
	WelcomeMsg      string      `toml:"welcome_message"`
	KeepaliveTime   int         `toml:"keepalive_time"`
	DataChanProxy   bool        `toml:"data_channel_proxy"`
	DataPortRange   string      `toml:"data_port_range"`
	MasqueradeIP    string      `toml:"masquerade_ip"`
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
			return nil, err
		}
	}

	if err := dataPortRangeValidation(c.DataPortRange); err != nil {
		logrus.Debug(err)
		c.DataPortRange = ""
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
}

func dataPortRangeValidation(r string) error {
	err := fmt.Errorf("Data port range config wrong. set default(random port)")
	portRange := strings.Split(r, "-")

	if len(portRange) != 2 {
		return err
	}

	min, err := strconv.Atoi(strings.TrimSpace(portRange[0]))
	if err != nil {
		return err
	}
	max, err := strconv.Atoi(strings.TrimSpace(portRange[1]))
	if err != nil {
		return err
	}

	// check each configs
	if min <= 0 || min > 65535 || max <= 0 || max > 65535 || min > max {
		return err
	}

	return nil
}
