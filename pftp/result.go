package pftp

import "github.com/sirupsen/logrus"

type result struct {
	code uint
	msg  string
	err  error
}

func (c *result) Response(handler *clientHandler) {
	if c.err != nil {
		logrus.Error(err)
	}

	if code != 0 {
		handler.writeMessage(c.code, c.msg)
	}
}
