package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pyama86/pftp/pftp"
	"github.com/sirupsen/logrus"
)

var ftpServer *pftp.FtpServer

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}
func main() {
	confFile := "./example.toml"
	logger := log.With(
		log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
		"ts", log.DefaultTimestampUTC,
		"caller", log.DefaultCaller,
	)

	if _, err := os.Stat(confFile); err != nil && os.IsNotExist(err) {
		level.Info(logger).Log("msg", "Not config file, creating one", "action", "conf_file.create", "confFile", confFile)

		if err := ioutil.WriteFile(confFile, confFileContent(), 0644); err != nil {
			level.Error(logger).Log("msg", "Couldn't create config file", "action", "conf_file.could_not_create", "confFile", confFile)
		}
	}

	if driver, err := NewSampleDriver("", confFile); err == nil {
		ftpServer := pftp.NewFtpServer(driver)
		if err := ftpServer.ListenAndServe(); err != nil {
			logrus.Fatal("msg", "Problem listening", "err", err)
		}
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

// MainDriver defines a very basic ftpserver driver
type MainDriver struct {
	Logger       log.Logger  // Logger
	SettingsFile string      // Settings file
	BaseDir      string      // Base directory from which to serve file
	tlsConfig    *tls.Config // TLS config (if applies)
	config       OurSettings // Our settings
	nbClients    int32       // Number of clients
}

// ClientDriver defines a very basic client driver
type ClientDriver struct {
	BaseDir string // Base directory from which to server file
}

// Account defines a user/pass password
type Account struct {
	User string // Username
	Pass string // Password
	Dir  string // Directory
}

// OurSettings defines our settings
type OurSettings struct {
	Server         pftp.Settings // Server settings (shouldn't need to be filled)
	Users          []Account     // Credentials
	MaxConnections int32         `toml:"max_connections"`
}

// GetSettings returns some general settings around the server setup
func (driver *MainDriver) GetSettings() (*pftp.Settings, error) {
	f, err := os.Open(driver.SettingsFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}
	//var config OurSettings
	if err := toml.Unmarshal(buf, &driver.config); err != nil {
		return nil, fmt.Errorf("problem loading \"%s\": %v", driver.SettingsFile, err)
	}
	if driver.config.Server.PublicHost == "" {
		level.Debug(driver.Logger).Log("msg", "Fetching our external IP address...")
		if driver.config.Server.PublicHost, err = externalIP(); err != nil {
			level.Warn(driver.Logger).Log("msg", "Couldn't fetch an external IP", "err", err)
		} else {
			level.Debug(driver.Logger).Log("msg", "Fetched our external IP address", "ipAddress", driver.config.Server.PublicHost)
		}
	}

	if len(driver.config.Users) == 0 {
		return nil, errors.New("you must have at least one user defined")
	}

	return &driver.config.Server, nil
}

// GetTLSConfig returns a TLS Certificate to use
func (driver *MainDriver) GetTLSConfig() (*tls.Config, error) {

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

// Live generation of a self-signed certificate
// This implementation of the driver doesn't load a certificate from a file on purpose. But it any proper implementation
// should most probably load the certificate from a file using tls.LoadX509KeyPair("cert_pub.pem", "cert_priv.pem").
func (driver *MainDriver) getCertificate() (*tls.Certificate, error) {
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

// WelcomeUser is called to send the very first welcome message
func (driver *MainDriver) WelcomeUser(cc pftp.ClientContext) (string, error) {
	nbClients := atomic.AddInt32(&driver.nbClients, 1)
	if nbClients > driver.config.MaxConnections {
		return "Cannot accept any additional client", fmt.Errorf("too many clients: %d > % d", driver.nbClients, driver.config.MaxConnections)
	}

	cc.SetDebug(true)
	// This will remain the official name for now
	return fmt.Sprintf(
			"Welcome on ftpserver, you're on dir %s, your ID is %d, your IP:port is %s, we currently have %d clients connected",
			driver.BaseDir,
			cc.ID(),
			cc.RemoteAddr(),
			nbClients),
		nil
}

// AuthUser authenticates the user and selects an handling driver
func (driver *MainDriver) AuthUser(cc pftp.ClientContext, user, pass string) (pftp.ClientHandlingDriver, error) {

	for _, act := range driver.config.Users {
		if act.User == user && act.Pass == pass {
			// If we are authenticated, we can return a client driver containing *our* basedir
			baseDir := driver.BaseDir + string(os.PathSeparator) + act.Dir
			os.MkdirAll(baseDir, 0777)
			return &ClientDriver{BaseDir: baseDir}, nil
		}
	}

	return nil, fmt.Errorf("could not authenticate you")
}

// UserLeft is called when the user disconnects, even if he never authenticated
func (driver *MainDriver) UserLeft(cc pftp.ClientContext) {
	atomic.AddInt32(&driver.nbClients, -1)
}

// ChangeDirectory changes the current working directory
func (driver *ClientDriver) ChangeDirectory(cc pftp.ClientContext, directory string) error {
	_, err := os.Stat(driver.BaseDir + directory)
	return err
}

// MakeDirectory creates a directory
func (driver *ClientDriver) MakeDirectory(cc pftp.ClientContext, directory string) error {
	return os.Mkdir(driver.BaseDir+directory, 0777)
}

// ListFiles lists the files of a directory
func (driver *ClientDriver) ListFiles(cc pftp.ClientContext) ([]os.FileInfo, error) {

	if cc.Path() == "/virtual" {
		files := make([]os.FileInfo, 0)
		files = append(files,
			virtualFileInfo{
				name: "localpath.txt",
				mode: os.FileMode(0666),
				size: 1024,
			},
			virtualFileInfo{
				name: "file2.txt",
				mode: os.FileMode(0666),
				size: 2048,
			},
		)
		return files, nil
	} else if cc.Path() == "/debug" {
		return make([]os.FileInfo, 0), nil
	}

	path := driver.BaseDir + cc.Path()

	files, err := ioutil.ReadDir(path)

	// We add a virtual dir
	if cc.Path() == "/" && err == nil {
		files = append(files, virtualFileInfo{
			name: "virtual",
			mode: os.FileMode(0666) | os.ModeDir,
			size: 4096,
		})
	}

	return files, err
}

// OpenFile opens a file in 3 possible modes: read, write, appending write (use appropriate flags)
func (driver *ClientDriver) OpenFile(cc pftp.ClientContext, path string, flag int) (pftp.FileStream, error) {

	if path == "/virtual/localpath.txt" {
		return &virtualFile{content: []byte(driver.BaseDir)}, nil
	}

	path = driver.BaseDir + path

	// If we are writing and we are not in append mode, we should remove the file
	if (flag & os.O_WRONLY) != 0 {
		flag |= os.O_CREATE
		if (flag & os.O_APPEND) == 0 {
			os.Remove(path)
		}
	}

	return os.OpenFile(path, flag, 0666)
}

// GetFileInfo gets some info around a file or a directory
func (driver *ClientDriver) GetFileInfo(cc pftp.ClientContext, path string) (os.FileInfo, error) {
	switch path {
	case "/virtual":
		return &virtualFileInfo{name: "virtual", size: 4096, mode: os.ModeDir}, nil
	case "/debug":
		return &virtualFileInfo{name: "debug", size: 4096, mode: os.ModeDir}, nil
	}

	path = driver.BaseDir + path

	return os.Stat(path)
}

// CanAllocate gives the approval to allocate some data
func (driver *ClientDriver) CanAllocate(cc pftp.ClientContext, size int) (bool, error) {
	return true, nil
}

// ChmodFile changes the attributes of the file
func (driver *ClientDriver) ChmodFile(cc pftp.ClientContext, path string, mode os.FileMode) error {
	path = driver.BaseDir + path

	return os.Chmod(path, mode)
}

// DeleteFile deletes a file or a directory
func (driver *ClientDriver) DeleteFile(cc pftp.ClientContext, path string) error {
	path = driver.BaseDir + path

	return os.Remove(path)
}

// RenameFile renames a file or a directory
func (driver *ClientDriver) RenameFile(cc pftp.ClientContext, from, to string) error {
	from = driver.BaseDir + from
	to = driver.BaseDir + to

	return os.Rename(from, to)
}

// NewSampleDriver creates a sample driver
func NewSampleDriver(dir string, settingsFile string) (*MainDriver, error) {
	if dir == "" {
		var err error
		dir, err = ioutil.TempDir("", "ftpserver")
		if err != nil {
			return nil, fmt.Errorf("could not find a temporary dir, err: %v", err)
		}
	}

	drv := &MainDriver{
		Logger:       log.NewNopLogger(),
		SettingsFile: settingsFile,
		BaseDir:      dir,
	}

	return drv, nil
}

// The virtual file is an example of how you can implement a purely virtual file
type virtualFile struct {
	content    []byte // Content of the file
	readOffset int    // Reading offset
}

func (f *virtualFile) Close() error {
	return nil
}

func (f *virtualFile) Read(buffer []byte) (int, error) {
	n := copy(buffer, f.content[f.readOffset:])
	f.readOffset += n
	if n == 0 {
		return 0, io.EOF
	}

	return n, nil
}

func (f *virtualFile) Seek(n int64, w int) (int64, error) {
	return 0, nil
}

func (f *virtualFile) Write(buffer []byte) (int, error) {
	return 0, nil
}

type virtualFileInfo struct {
	name string
	size int64
	mode os.FileMode
}

func (f virtualFileInfo) Name() string {
	return f.name
}

func (f virtualFileInfo) Size() int64 {
	return f.size
}

func (f virtualFileInfo) Mode() os.FileMode {
	return f.mode
}

func (f virtualFileInfo) IsDir() bool {
	return f.mode.IsDir()
}

func (f virtualFileInfo) ModTime() time.Time {
	return time.Now().UTC()
}

func (f virtualFileInfo) Sys() interface{} {
	return nil
}

func externalIP() (string, error) {
	// If you need to take a bet, amazon is about as reliable & sustainable a service as you can get
	rsp, err := http.Get("http://checkip.amazonaws.com")
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()

	buf, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return "", err
	}

	return string(bytes.TrimSpace(buf)), nil
}

func confFileContent() []byte {
	str := `# ftpserver configuration file
#
# These are all the config parameters with their default values. If not present,

# Max number of control connections to accept
# max_connections = 0
max_connections = 10

[server]
# Address to listen on
# listen_host = "0.0.0.0"

# Port to listen on
# listen_port = 2121

# Public host to expose in the passive connection
# public_host = ""

# Idle timeout time
# idle_timeout = 900

# Data port range from 10000 to 15000
# [dataPortRange]
# start = 2122
# end = 2200

[server.dataPortRange]
start = 2122
end = 2200

[[users]]
user="fclairamb"
pass="floflo"
dir="shared"

[[users]]
user="test"
pass="test"
dir="shared"

[[users]]
user="mcardon"
pass="marmar"
dir="marie"
`
	return []byte(str)
}
