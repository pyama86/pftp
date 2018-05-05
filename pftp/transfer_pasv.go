package pftp

import (
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type transferHandler interface {
	Open() (*ProxyServer, error)
	Close() error
}

type passiveTransferHandler struct {
	listener           net.Listener
	tcpListener        *net.TCPListener
	Port               int
	originTransferPort int
	originAddr         string
	proxyServer        *ProxyServer
}

func (c *clientHandler) handlePASV() *result {
	response, err := c.controleProxy.SendAndReadToOrigin(c.line)
	if err != nil {
		return &result{
			err: err,
		}
	}

	// origin server listen port
	assined := regexp.MustCompile(`.+\(\|\|\|([0-9]+)\|\)`)
	originTransferPort := assined.FindSubmatch([]byte(response))
	originPort := 0
	if originTransferPort == nil {
		assined = regexp.MustCompile(`.+\(\d+,\d+,\d+,\d+,(\d+),(\d+)\)`)
		originTransferPort = assined.FindSubmatch([]byte(response))
		if originTransferPort == nil {
			// 2回目以降の接続はポートが通知されない
			if response[0:3] == "150" {
				return &result{
					code: 150,
					msg:  response[4:],
				}
			}

			return &result{
				err: fmt.Errorf("pasv mode port unmatch: %s", response),
			}
		}

		five, err := strconv.Atoi(string(originTransferPort[1]))
		if err != nil {
			return &result{
				err: err,
			}
		}

		six, err := strconv.Atoi(string(originTransferPort[2]))
		if err != nil {
			return &result{
				err: err,
			}
		}
		o := five*256 + six
		originPort = o
	} else {
		o, err := strconv.Atoi(string(originTransferPort[1]))
		if err != nil {
			return &result{
				err: err,
			}
		}
		originPort = o
	}

	addr, _ := net.ResolveTCPAddr("tcp", ":0")
	var tcpListener *net.TCPListener

	portRange := c.config.DataPortRange

	// choice proxy port
	if portRange != nil {
		for start := portRange.Start; start < portRange.End; start++ {
			port := portRange.Start + rand.Intn(portRange.End-portRange.Start)
			laddr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:"+fmt.Sprintf("%d", port))
			if err != nil {
				continue
			}

			tcpListener, err = net.ListenTCP("tcp", laddr)
			if err == nil {
				break
			}
		}

	} else {
		tcpListener, err = net.ListenTCP("tcp", addr)
	}

	if err != nil {
		return &result{
			err: err,
		}
	}

	var listener net.Listener
	if c.transferTLS && c.config.TLSConfig != nil {
		listener = tls.NewListener(tcpListener, c.config.TLSConfig)
	} else {
		listener = tcpListener
	}

	p := &passiveTransferHandler{
		tcpListener:        tcpListener,
		listener:           listener,
		Port:               tcpListener.Addr().(*net.TCPAddr).Port,
		originTransferPort: originPort,
		originAddr:         strings.Split(c.controleProxy.origin.RemoteAddr().String(), ":")[0],
	}

	var r *result
	if c.command == "PASV" {
		p1 := p.Port / 256
		p2 := p.Port - (p1 * 256)
		ip := strings.Split(c.conn.LocalAddr().String(), ":")[0]
		quads := strings.Split(ip, ".")

		r = &result{
			code: 227,
			msg:  fmt.Sprintf("Entering Passive Mode (%s,%s,%s,%s,%d,%d)", quads[0], quads[1], quads[2], quads[3], p1, p2),
		}
	} else {
		r = &result{
			code: 229,
			msg:  fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", p.Port),
		}
	}
	c.transfer = p
	return r
}

func (p *passiveTransferHandler) ConnectionWait(wait time.Duration) (*ProxyServer, error) {
	if p.proxyServer == nil {
		p.tcpListener.SetDeadline(time.Now().Add(wait))
		var err error
		connection, err := p.listener.Accept()
		if err != nil {
			return nil, err
		}

		proxy, err := NewProxyServer(60, connection, p.originAddr+":"+strconv.Itoa(p.originTransferPort))

		if err != nil {
			return nil, err
		}
		p.proxyServer = proxy
	}

	return p.proxyServer, nil
}

func (p *passiveTransferHandler) Open() (*ProxyServer, error) {
	return p.ConnectionWait(time.Minute)
}

func (p *passiveTransferHandler) Close() error {
	if p.tcpListener != nil {
		p.tcpListener.Close()
	}

	if p.proxyServer != nil {
		p.proxyServer.Close()
		p.proxyServer = nil
	}
	return nil
}
