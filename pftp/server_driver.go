package pftp

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	"sync/atomic"

	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"

	"github.com/pyama86/ftpserver/server"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/naoina/toml"
)

type ServerDriver struct {
	Logger         log.Logger  // Logger
	configFile     string      // Settings file
	BaseDir        string      // Base directory from which to serve file
	tlsConfig      *tls.Config // TLS config (if applies)
	config         OurSettings // Our settings
	nbClients      int32       // Number of clients
	AuthMiddleware Auther
}

type OurSettings struct {
	Server         server.Settings
	BaseDir        string
	MaxConnections int32
}

func NewDriver(configFile string) (*ServerDriver, error) {
	drv := &ServerDriver{
		configFile: configFile,
	}

	return drv, nil
}

func (driver *ServerDriver) AuthUser(cc server.ClientContext, user, pass string) (server.ClientHandlingDriver, error) {
	actDir, err := driver.AuthMiddleware.Auth(user, pass)
	if err == nil {
		baseDir := path.Join(driver.BaseDir, actDir)
		os.MkdirAll(baseDir, 0777)
		return &ClientDriver{BaseDir: baseDir}, nil
	}
	return nil, fmt.Errorf("could not authenticate you")
}

func (driver *ServerDriver) GetSettings() (*server.Settings, error) {
	f, err := os.Open(driver.configFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	if err := toml.Unmarshal(buf, &driver.config); err != nil {
		return nil, fmt.Errorf("problem loading \"%s\": %v", driver.configFile, err)
	}

	driver.BaseDir = driver.config.BaseDir
	return &driver.config.Server, nil
}

func (driver *ServerDriver) GetTLSConfig() (*tls.Config, error) {
	if driver.tlsConfig == nil {
		level.Info(driver.Logger).Log("msg", "Loading certificate")
		if cert, err := driver.getCertificate(); err == nil {
			driver.tlsConfig = &tls.Config{
				NextProtos:   []string{"ftp"},
				Certificates: []tls.Certificate{*cert},
			}
		} else {
			return nil, err
		}
	}
	return driver.tlsConfig, nil
}

func (driver *ServerDriver) getCertificate() (*tls.Certificate, error) {
	level.Info(driver.Logger).Log("msg", "Creating certificate")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)

	if err != nil {
		level.Error(driver.Logger).Log("msg", "Could not generate key", "err", err)
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
		IsCA:        false,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)

	if err != nil {
		level.Error(driver.Logger).Log("msg", "Could not create cert", "err", err)
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

func (driver *ServerDriver) WelcomeUser(cc server.ClientContext) (string, error) {
	nbClients := atomic.AddInt32(&driver.nbClients, 1)
	if nbClients > driver.config.MaxConnections {
		return "Cannot accept any additional client", fmt.Errorf("too many clients: %d > % d", driver.nbClients, driver.config.MaxConnections)
	}

	cc.SetDebug(true)
	return fmt.Sprintf(
			"Welcome on ftp, you're on dir %s, your ID is %d, your IP:port is %s, we currently have %d clients connected",
			driver.BaseDir,
			cc.ID(),
			cc.RemoteAddr(),
			nbClients),
		nil
}

func (driver *ServerDriver) UserLeft(cc server.ClientContext) {
	atomic.AddInt32(&driver.nbClients, -1)
}
