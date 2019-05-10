package pftp

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	DATA_TRANSFER_BUFFER_SIZE = 1024
	LISTENER_TIMEOUT          = 30
)

type dataListener struct {
	responseCode string
	listener     net.Listener
	clientConn   net.Conn
	originConn   net.Conn
	originAddr   string
	originIP     string
	originPort   string
	timeout      int
	isConnected  bool
	listenPort   string
	log          *logger
}

// Make listener for data connection
func newDataListener(line string, originAddr string, timeout int, log *logger, resCode string) (string, int, error) {
	d := &dataListener{
		responseCode: resCode,
		originAddr:   originAddr,
		listener:     nil,
		clientConn:   nil,
		originConn:   nil,
		timeout:      timeout,
		isConnected:  false,
		log:          log,
	}
	d.getPasvIPPort(line)

	l, err := net.Listen("tcp4", d.originIP+":")
	if err != nil {
		d.log.err("cannot open data channel port: %v", err)
		return "", -1, err
	}

	d.listener = l
	d.listenPort = strings.SplitN(l.Addr().String(), ":", 2)[1]
	listenPort, _ := strconv.Atoi(d.listenPort)

	// start listener & listener timer
	go d.startDataListener()
	go d.setListenerTimer()

	return d.originIP, listenPort, nil
}

// parse PASV IP and Port from server response
func (d *dataListener) getPasvIPPort(line string) {
	var ip string
	var port int

	switch d.responseCode {
	case "PASV":
		pasvAddr := strings.Split(strings.Trim(strings.Split(strings.Trim(line, ").\r\n"), " ")[4], "("), ",")
		ip = ""
		for i := 0; i < 4; i++ {
			ip += pasvAddr[i]
			if i < 3 {
				ip += "."
			}
		}

		port = 0
		for i := 4; i <= 5; i++ {
			n, _ := strconv.Atoi(pasvAddr[i])
			if i == 4 {
				n *= 256
			}
			port += n
		}
	case "EPSV":
		ip = strings.Split(d.originAddr, ":")[0]

		pasvPort := strings.Trim(strings.Split(strings.Trim(line, ").\r\n"), " ")[5], "()|")
		port, _ = strconv.Atoi(pasvPort)
	}

	d.originIP = ip
	d.originPort = strconv.Itoa(port)
	return
}

// Make listener for data connection
func (d *dataListener) startDataListener() error {
	conn, err := d.listener.Accept()
	if err != nil {
		return err
	}

	logrus.Info("Data channel connected : ", "clientIp ", conn.RemoteAddr())

	// set conn to TCPConn
	tcpConn := conn.(*net.TCPConn)

	if d.timeout > 0 {
		// set linger 0 and tcp keepalive setting between client connection
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(time.Duration(d.timeout) * time.Second)
		tcpConn.SetLinger(0)

		// set deadline
		tcpConn.SetDeadline(time.Now().Add(time.Duration(d.timeout) * time.Second))
	}
	d.clientConn = conn

	d.isConnected = true

	err = d.connectToOriginDataChan()
	if err != nil {
		return err
	}

	clientToOriginDone := make(chan struct{})
	OriginToClientDone := make(chan struct{})

	// client to origin
	go d.dataTransfer(d.clientConn, d.originConn, clientToOriginDone)

	// origin to client
	go d.dataTransfer(d.originConn, d.clientConn, OriginToClientDone)

	// wait until data transfer is completely done
	<-clientToOriginDone
	<-OriginToClientDone

	// close listener
	d.listener.Close()
	d.listener = nil

	d.log.debug("Data channel disconnected")

	return nil
}

// Connect to origin server data channel
func (d *dataListener) connectToOriginDataChan() error {
	conn, err := net.Dial("tcp", d.originIP+":"+d.originPort)
	if err != nil {
		return err
	}

	// set conn to TCPConn
	tcpConn := conn.(*net.TCPConn)

	// set linger 0 to local origin ftp server
	// set linger 0 and tcp keepalive setting between client connection
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(time.Duration(d.timeout) * time.Second)
	tcpConn.SetLinger(0)

	// set deadline
	tcpConn.SetDeadline(time.Now().Add(time.Duration(d.timeout) * time.Second))

	d.originConn = conn

	return nil
}

// check data channel connected and close listener if listening over 30 seconds
func (d *dataListener) setListenerTimer() {
	for i := 0; i < LISTENER_TIMEOUT; i++ {
		if d.isConnected {
			return
		}
		time.Sleep(time.Duration(1) * time.Second)
	}

	if !d.isConnected {
		if d.listener != nil {
			d.listener.Close()
			d.listener = nil
			d.log.debug("Data channel disconnected by timeout: %s", d.listener.Addr().String())
		}
	}

	return
}

// send data until got EOF
func (d *dataListener) dataTransfer(reader net.Conn, writer net.Conn, done chan struct{}) {
	buffer := make([]byte, DATA_TRANSFER_BUFFER_SIZE)

	for {
		if _, err := reader.Read(buffer); err != nil {
			break
		}
		if _, err := writer.Write(buffer); err != nil {
			break
		}
	}
	reader.Close()
	writer.Close()

	done <- struct{}{}

	return
}
