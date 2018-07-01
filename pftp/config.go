package pftp

import (
	"crypto/tls"

	"github.com/BurntSushi/toml"
)

type portRange struct {
	Start int
	End   int
}
type config struct {
	ListenAddr               string      `toml:"listen_addr"`
	RemoteAddr               string      `toml:"remote_addr"`
	IdleTimeout              int         `toml:"idle_timeout"`
	ProxyTimeout             int         `toml:"proxy_timeout"`
	DataConnectionTimeout    int         `toml:"data_connection_timeout"`
	DataPortRange            *portRange  `toml:"port_range"`
	MaxConnections           int32       `toml:"max_connections"`
	tls                      *tlsPair    `toml:"tls"`
	TLSConfig                *tls.Config `toml:"-"`
	UseUnknownActiveDataPort bool        `toml:"use_unknown_active_dataport"`
}

type tlsPair struct {
	cert string
	key  string
}

func loadConfig(path string) (*config, error) {
	var c config
	defaultConfig(&c)

	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, err
	}

	if c.tls != nil {
		if cert, err := tls.LoadX509KeyPair(c.tls.cert, c.tls.key); err == nil {
			c.TLSConfig = &tls.Config{
				NextProtos:   []string{"ftp"},
				Certificates: []tls.Certificate{cert},
			}
		} else {
			return nil, err
		}
	}

	return &c, nil
}

func defaultConfig(config *config) {
	config.ListenAddr = "0.0.0.0:2121"
	config.IdleTimeout = 900
	config.ProxyTimeout = 900
	config.UseUnknownActiveDataPort = true
}
