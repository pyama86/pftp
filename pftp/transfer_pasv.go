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

	"github.com/sirupsen/logrus"
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

func (c *clientHandler) handlePASV() {
	response, err := c.controleProxy.SendAndReadToOrigin(c.line)
	if err != nil {
		logrus.Error(err)
		return
	}

	// origin server listen port
	assined := regexp.MustCompile(`.+\(\|\|\|([0-9]+)\|\)`)
	originTransferPort := assined.FindSubmatch([]byte(response))
	originPort := 0
	if originTransferPort == nil {
		assined = regexp.MustCompile(`.+\(\d+,\d+,\d+,\d+,(\d+),(\d+)\)`)
		originTransferPort = assined.FindSubmatch([]byte(response))
		if originTransferPort == nil {
			logrus.Errorf("pasv mode port unmatch: %s", response)
			return
		}

		five, err := strconv.Atoi(string(originTransferPort[1]))
		if err != nil {
			logrus.Error(err)
			return
		}

		six, err := strconv.Atoi(string(originTransferPort[2]))
		if err != nil {
			logrus.Error(err)
			return
		}
		o := five*256 + six
		originPort = o
	} else {
		o, err := strconv.Atoi(string(originTransferPort[1]))
		if err != nil {
			logrus.Error(err)
			return
		}
		originPort = o
	}

	addr, _ := net.ResolveTCPAddr("tcp", ":0")
	var tcpListener *net.TCPListener

	portRange := c.daddy.config.DataPortRange

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
		logrus.Error("Could not listen")
		return
	}

	var listener net.Listener
	if c.transferTLS && c.daddy.config.TLSConfig != nil {
		listener = tls.NewListener(tcpListener, c.daddy.config.TLSConfig)
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

	if c.command == "PASV" {
		p1 := p.Port / 256
		p2 := p.Port - (p1 * 256)
		ip := strings.Split(c.conn.LocalAddr().String(), ":")[0]
		quads := strings.Split(ip, ".")

		c.writeMessage(227, fmt.Sprintf("Entering Passive Mode (%s,%s,%s,%s,%d,%d)", quads[0], quads[1], quads[2], quads[3], p1, p2))
	} else {
		c.writeMessage(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", p.Port))
	}
	c.transfer = p

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
