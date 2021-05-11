package test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// GetCertificate method is quoted
// https://github.com/fclairamb/ftpserver
func GetCertificate() (*tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1337),
		Subject: pkix.Name{
			CommonName:   "localhost",
			Organization: []string{"FTPServer"},
		},
		DNSNames:              []string{"localhost"},
		SignatureAlgorithm:    x509.SHA256WithRSA,
		PublicKeyAlgorithm:    x509.RSA,
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour * 24 * 7),
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		BasicConstraintsValid: true,
		IsCA:                  false,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)

	if err != nil {
		return nil, err
	}

	var certPem, keyPem bytes.Buffer
	if err := pem.Encode(&certPem, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return nil, err
	}
	if err := pem.Encode(&keyPem, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return nil, err
	}
	c, err := tls.X509KeyPair(certPem.Bytes(), keyPem.Bytes())
	return &c, err
}

func launchTestServer(t *testing.T) net.Listener {
	s, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// LaunchTestServer Launch test server
func LaunchTestServer(server *net.Listener, conn chan net.Conn, done chan struct{}, serverready chan struct{}, t *testing.T) {
	*server = launchTestServer(t)
	defer (*server).Close()

	serverready <- struct{}{}
	for {
		c, err := (*server).Accept()
		if err != nil {
			if !strings.Contains(err.Error(), "use of closed network connection") {
				t.Fatal(err)
			}
			break
		}

		conn <- c
	}
	done <- struct{}{}
}
