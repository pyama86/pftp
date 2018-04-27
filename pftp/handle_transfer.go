package pftp

import (
	"errors"
	"fmt"
	"io"
)

func (c *clientHandler) TransferOpen() (*ProxyServer, error) {
	if c.transfer == nil {
		return nil, errors.New("no passive connection declared")
	}
	c.writeMessage(150, "Using transfer connection")

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

func (c *clientHandler) handleSTOR() *result {
	return c.transferFile()
}

func (c *clientHandler) handleAPPE() *result {
	return c.transferFile()
}

func (c *clientHandler) handleRETR() *result {
	return c.transferFile()
}

func (c *clientHandler) transferFile() *result {
	var err error
	var proxy *ProxyServer

	if err := c.controleProxy.SendToOrigin(c.line); err != nil {
		return &result{
			code: 500,
			msg:  "Could not transfer file: " + err.Error(),
			err:  err,
		}
	}

	if proxy, err = c.TransferOpen(); err == nil {
		defer c.TransferClose()
		err = c.transferWithCommandProxy(proxy)
	}

	if err != nil {
		return &result{
			code: 550,
			msg:  "Could not transfer file: " + err.Error(),
			err:  err,
		}
	}
	return nil
}

func (c *clientHandler) transferWithCommandProxy(proxy *ProxyServer) error {
	// データ転送の完了はシリアルに待つ
	if err := proxy.Start(); err != nil && err != io.EOF {
		return err
	}

	for {
		// オリジンサーバから完了通知を受け取る
		res, err := c.controleProxy.ReadFromOrigin()
		if err != nil {
			return err
		}
		r1 := string(res[0])
		if r1 != `1` {
			// クライアントに完了通知を送る
			err = c.controleProxy.SendToClient(res)
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (c *clientHandler) handleLIST() *result {
	var err error
	var proxy *ProxyServer
	c.controleProxy.SendToOrigin(c.line)

	if proxy, err = c.TransferOpen(); err == nil {
		defer c.TransferClose()
		err = c.transferWithCommandProxy(proxy)
	}

	if err != nil {
		return &result{
			code: 500,
			msg:  fmt.Sprintf("Could not list: %v", err),
			err:  err,
		}
	}
	return nil
}
