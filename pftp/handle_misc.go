package pftp

import (
	"fmt"
	"strings"
)

func (c *clientHandler) handleFEAT() *result {
	if err := c.controleProxy.SendToOriginWithProxy(c.line); err != nil {
		return &result{
			code: 500,
			msg:  fmt.Sprintf("Could not feat: %v", err),
			err:  err,
		}
	}

	for {
		b, err := c.controleProxy.ReadFromOrigin()
		if err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Could not feat: %v", err),
				err:  err,
			}
		}

		if err := c.controleProxy.SendToClient(b); err != nil {
			return &result{
				code: 500,
				msg:  fmt.Sprintf("Could not feat: %v", err),
				err:  err,
			}
		}

		if strings.Index(strings.ToUpper(b), " END") > 0 || string(b[0]) == "5" {
			return nil
		}
	}
	return nil
}

func (c *clientHandler) handlePROT() {
	c.transferTLS = c.param == "P"
	c.controleProxy.SendToOriginWithProxy(c.line)
}
