package pftp

import "github.com/sirupsen/logrus"

type result struct {
	code int
	msg  string
	err  error
}

func (r *result) Response(handler *clientHandler) {
	if r.err != nil {
		logrus.Error(r.err)
	}

	if r.code != 0 {
		handler.writeMessage(r.code, r.msg)
	}
}
