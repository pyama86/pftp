package pftp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strings"
)

// TLS version codes
const (
	SSLv3  = 0x0300
	TLSv1  = 0x0301
	TLSv11 = 0x0302
	TLSv12 = 0x0303
)

// get TLS protocol from string version name
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

type tlsData struct {
	rootCA *x509.CertPool
	cert   *tls.Certificate
	config *tls.Config
}

// tls configset for client and origin
type tlsDataSet struct {
	forClient *tlsData
	forOrigin *tlsData
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
	caCertPEM, err := ioutil.ReadFile(caCertFile)
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
		ClientCAs:                caCert,
		ClientAuth:               tls.VerifyClientCertIfGiven,
		CipherSuites:             getCiphers(TLS.CipherSuite),
		PreferServerCipherSuites: true,
		VerifyConnection:         t.verifyTLSConnection,
	}

	return t, nil
}

// verify TLS connection using Peer certificates
func (t *tlsData) verifyTLSConnection(cs tls.ConnectionState) error {
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
			return fmt.Errorf("varidate error: %v", err)
		}
	}

	return nil
}

// get tls config
func (t *tlsData) getTLSConfig() *tls.Config {
	return t.config
}

// set specific tls version to tls.Config
func (t *tlsData) setSpecificTLSVersion(version uint16) {
	t.config.MinVersion = version
	t.config.MaxVersion = version
}

// set server name to tls.Config
func (t *tlsData) setServerName(name string) {
	t.config.ServerName = name
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
