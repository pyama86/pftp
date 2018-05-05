package pftp

import (
	"errors"
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

func (c *clientHandler) handleSTOR() *result {
	return c.upload()
}

func (c *clientHandler) handleAPPE() *result {
	return c.upload()
}

func (c *clientHandler) handleRETR() *result {
	return c.download()
}

func (c *clientHandler) upload() *result {
	return c.transferFile(true)
}

func (c *clientHandler) download() *result {
	return c.transferFile(false)
}
func (c *clientHandler) transferFile(isUpload bool) *result {
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
		err = proxy.Start(isUpload)
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

func (c *clientHandler) handleLIST() *result {
	return c.download()
}
