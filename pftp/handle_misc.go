package pftp

import (
	"strings"

	"github.com/sirupsen/logrus"
)

func (c *clientHandler) handleFEAT() {
	c.controlProxy.SendToOriginWithProxy(c.line)
	for {
		b, err := c.controlProxy.ReadFromOrigin()
		if err != nil {
			logrus.Error(err)
			return
		}

		if err := c.controlProxy.SendToClient(b); err != nil {
			logrus.Error(err)
			return
		}

		if strings.Index(strings.ToUpper(b), " END") > 0 || string(b[0]) == "5" {
			return
		}
	}
}

func (c *clientHandler) handlePROT() {
	c.transferTLS = c.param == "P"
	c.controlProxy.SendToOriginWithProxy(c.line)
}
