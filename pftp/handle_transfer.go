package pftp

import (
	"errors"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

func (c *clientHandler) TransferOpen() (*ProxyServer, error) {
	if c.transfer == nil {
		return nil, errors.New("no passive connection declared")
	}
	conn, err := c.transfer.Open()
	if err != nil {
		return nil, err
	}
	return conn, err
}

func (c *clientHandler) TransferClose() {
	if c.transfer != nil {
		c.transfer.Close()
		c.transfer = nil
	}
}

func (c *clientHandler) handleSTOR() {
	c.transferFile()
}

func (c *clientHandler) handleAPPE() {
	c.transferFile()
}

func (c *clientHandler) handleRETR() {
	c.transferFile()
}

func (c *clientHandler) transferFile() {
	var err error
	var proxy *ProxyServer
	c.controlProxy.SendToOrigin(c.line)
	if proxy, err = c.TransferOpen(); err == nil {
		defer c.TransferClose()
		err = c.transferWithCommandProxy(proxy)
	}

	if err != nil {
		logrus.Error(err)
		c.writeMessage(550, "Could not transfer file: "+err.Error())
		return
	}

}

func (c *clientHandler) transferWithCommandProxy(proxy *ProxyServer) error {
	// データ転送の完了はシリアルに待つ
	err := proxy.Start()
	if err != io.EOF {
		return err
	}

	for {
		// オリジンサーバから完了通知を受け取る
		res, err := c.controlProxy.ReadFromOrigin()
		if err != nil {
			return err
		}

		r1 := string(res[0])
		if r1 != `1` {
			// クライアントに完了通知を送る
			err = c.controlProxy.SendToClient(res)
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (c *clientHandler) handleLIST() {
	var err error
	var proxy *ProxyServer
	c.controlProxy.SendToOrigin(c.line)
	c.writeMessage(150, "Using transfer connection")

	if proxy, err = c.TransferOpen(); err == nil {
		defer c.TransferClose()
		err = c.transferWithCommandProxy(proxy)

	}

	if err != nil {
		logrus.Error(err)
		c.writeMessage(500, fmt.Sprintf("Could not list: %v", err))
	}
}
