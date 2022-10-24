package pftp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// TLS version codes
const (
	defaultTLSVer = "TLSv1.2"
)

// get TLS protocol from string version name
func getTLSProtocol(protocol string) uint16 {
	switch protocol {
	case "TLSv1":
		return tls.VersionTLS10
	case "TLSv1.1":
		return tls.VersionTLS11
	case "TLSv1.2":
		return tls.VersionTLS12
	case "TLSv1.3":
		return tls.VersionTLS13
	default:
		logrus.Debugf("%s is an unsupported TLS protocol version. use default: %s", protocol, defaultTLSVer)
		return tls.VersionTLS12 // the default TLS protocol is TLSv1.2
	}
}

// get TLS protocol name from uint16 id
func getTLSProtocolName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLSv1"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS13:
		return "TLSv1.3"
	default:
		return "unsupported TLS version"
	}
}

type tlsData struct {
	rootCA *x509.CertPool
	cert   *tls.Certificate
	config *tls.Config
	mutex  sync.Mutex
}

// tls configset for client and origin
type tlsDataSet struct {
	forClient   *tlsData
	forOrigin   *tlsData
	version     uint16
	cipherSuite uint16
	serverName  string
}

// build origin side tls config
// it is working TLS client
func buildTLSConfigForOrigin() *tlsData {
	return &tlsData{
		config: &tls.Config{
			InsecureSkipVerify:     true,
			ClientSessionCache:     tls.NewLRUClientSessionCache(10),
			SessionTicketsDisabled: false,
		},
		rootCA: nil,
		cert:   nil,
	}
}

// build client side tls config (pftp works like server)
// it is working TLS server
func buildTLSConfigForClient(TLS *tlsPair) (*tlsData, error) {
	var t *tlsData

	caCertFile := TLS.CACert

	if len(caCertFile) == 0 {
		caCertFile = TLS.Cert
	}
	caCertPEM, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, err
	}

	caCert := x509.NewCertPool()
	ok := caCert.AppendCertsFromPEM(caCertPEM)
	if !ok {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	cert, err := tls.LoadX509KeyPair(TLS.Cert, TLS.Key)
	if err != nil {
		return nil, fmt.Errorf("TLS configuration error: %s", err.Error())
	}

	t = &tlsData{
		config: nil,
		rootCA: caCert,
		cert:   &cert,
	}

	t.config = &tls.Config{
		NextProtos:               []string{"ftp"},
		Certificates:             []tls.Certificate{cert},
		MinVersion:               getTLSProtocol(TLS.MinProtocol),
		MaxVersion:               getTLSProtocol(TLS.MaxProtocol),
		CipherSuites:             getCiphers(TLS.CipherSuite),
		PreferServerCipherSuites: true,
		VerifyConnection:         t.verifyTLSConnection,
	}

	return t, nil
}

// verify TLS connection using Peer certificates
func (t *tlsData) verifyTLSConnection(cs tls.ConnectionState) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	opts := x509.VerifyOptions{
		Roots:         t.rootCA,
		DNSName:       cs.ServerName,
		Intermediates: x509.NewCertPool(),
	}

	if len(cs.PeerCertificates) > 0 {
		if len(cs.PeerCertificates) >= 2 {
			for _, cert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(cert)
			}
		}

		_, err := cs.PeerCertificates[0].Verify(opts)
		if err != nil {
			return fmt.Errorf("variation error: %v", err)
		}
	}

	return nil
}

// get tls config
func (t *tlsData) getTLSConfig() *tls.Config {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.config
}

// set specific tls version to tls.Config
func (t *tlsData) setSpecificTLSVersion(tlsVersion uint16) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.config.MinVersion = tlsVersion
	t.config.MaxVersion = tlsVersion
}

// set server name to tls.Config
func (t *tlsData) setServerName(name string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.config.ServerName = name
}

// set specific cipher suite name to tls.Config
func (t *tlsData) setCipherSUite(cipherSuite uint16) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.config.CipherSuites = []uint16{cipherSuite}
}

// get available Ciphersuites from config
func getCiphers(ciphers string) []uint16 {
	cipherNames := strings.Split(ciphers, ":")

	var result []uint16

	for _, cipherName := range removeDuplicates(cipherNames) {
		for _, c := range tls.CipherSuites() {
			if c.Name == strings.TrimSpace(cipherName) {
				result = append(result, c.ID)
			}
		}
	}

	return result
}

// remove duplicate ciphersuites from config
func removeDuplicates(params []string) []string {
	if len(params) == 0 {
		return params
	}

	var result []string
	exist := make(map[string]bool)

	for _, param := range params {
		if _, ok := exist[param]; !ok {
			result = append(result, param)
		}
		exist[param] = true
	}

	return result
}
