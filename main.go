package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

var ftpServer *pftp.FtpServer

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.Info("hoge")
}
func main() {
	confFile := "./example.toml"

	ftpServer := pftp.NewFtpServer(confFile)
	if err := ftpServer.ListenAndServe(); err != nil {
		logrus.Fatal("msg", "Problem listening", "err", err)
	}
}

func signalHandler() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGTERM)
	for {
		switch <-ch {
		case syscall.SIGTERM:
			ftpServer.Stop()
			break
		}
	}
}

// GetTLSConfig returns a TLS Certificate to use
//func (driver *MainDriver) GetTLSConfig() (*tls.Config, error) {
//
//	if driver.tlsConfig == nil {
//		level.Info(driver.Logger).Log("msg", "Loading certificate")
//		if cert, err := driver.getCertificate(); err == nil {
//			driver.tlsConfig = &tls.Config{
//				NextProtos:   []string{"ftp"},
//				Certificates: []tls.Certificate{*cert},
//			}
//		} else {
//			return nil, err
//		}
//	}
//	return driver.tlsConfig, nil
//}

// Live generation of a self-signed certificate
// This implementation of the driver doesn't load a certificate from a file on purpose. But it any proper implementation
// should most probably load the certificate from a file using tls.LoadX509KeyPair("cert_pub.pem", "cert_priv.pem").
//func (driver *MainDriver) getCertificate() (*tls.Certificate, error) {
//	level.Info(driver.Logger).Log("msg", "Creating certificate")
//	priv, err := rsa.GenerateKey(rand.Reader, 2048)
//
//	if err != nil {
//		level.Error(driver.Logger).Log("msg", "Could not generate key", "err", err)
//		return nil, err
//	}
//
//	now := time.Now().UTC()
//
//	template := &x509.Certificate{
//		SerialNumber: big.NewInt(1337),
//		Subject: pkix.Name{
//			CommonName:   "localhost",
//			Organization: []string{"FTPServer"},
//		},
//		DNSNames:              []string{"localhost"},
//		SignatureAlgorithm:    x509.SHA256WithRSA,
//		PublicKeyAlgorithm:    x509.RSA,
//		NotBefore:             now.Add(-time.Hour),
//		NotAfter:              now.Add(time.Hour * 24 * 7),
//		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
//		BasicConstraintsValid: true,
//		IsCA:        false,
//		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
//		KeyUsage:    x509.KeyUsageDigitalSignature,
//	}
//	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
//
//	if err != nil {
//		level.Error(driver.Logger).Log("msg", "Could not create cert", "err", err)
//		return nil, err
//	}
//
//	var certPem, keyPem bytes.Buffer
//	if err := pem.Encode(&certPem, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
//		return nil, err
//	}
//	if err := pem.Encode(&keyPem, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
//		return nil, err
//	}
//	c, err := tls.X509KeyPair(certPem.Bytes(), keyPem.Bytes())
//	return &c, err
//}
