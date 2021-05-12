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
	config *tls.Config
}

// tls configset for client and origin
type tlsDataSet struct {
	forClient *tlsData
	forOrigin *tlsData
}

// get tls config for client connection
func (t *tlsDataSet) getTLSConfigForClient() *tls.Config {
	return t.forClient.config
}

// get tls config for origin connection
func (t *tlsDataSet) getTLSConfigForOrigin() *tls.Config {
	return t.forOrigin.config
}

// build origin side tls config (pftp works like client)
func buildTLSConfigForOrigin() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify:     true,
		ClientSessionCache:     tls.NewLRUClientSessionCache(10),
		SessionTicketsDisabled: false,
	}
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

// build client side tls config (pftp works like server)
func buildTLSConfigForClient(TLS *tlsPair) (*tls.Config, error) {
	var tlsConfig *tls.Config

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

	if cert, err := tls.LoadX509KeyPair(TLS.Cert, TLS.Key); err == nil {
		tlsConfig = &tls.Config{
			NextProtos:               []string{"ftp"},
			Certificates:             []tls.Certificate{cert},
			MinVersion:               getTLSProtocol(TLS.MinProtocol),
			MaxVersion:               getTLSProtocol(TLS.MaxProtocol),
			ClientCAs:                caCert,
			ClientAuth:               tls.VerifyClientCertIfGiven,
			PreferServerCipherSuites: true,
			CipherSuites:             getCiphers(TLS.CipherSuite),
			VerifyConnection:         verifyTLSConnection,
		}
	} else {
		return nil, fmt.Errorf("TLS configuration error: %s", err.Error())
	}

	return tlsConfig, nil
}

// verify TLS connection using Peer certificates
func verifyTLSConnection(cs tls.ConnectionState) error {
	opts := x509.VerifyOptions{
		DNSName:       cs.ServerName,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
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
