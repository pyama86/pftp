package pftp

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

type logger struct {
	fromip string
	user   string
	id     int
}

func (l *logger) debug(format string, args ...interface{}) {
	format = fmt.Sprintf("[%d] user:%s addr:%s %s", l.id, l.user, l.fromip, format)
	logrus.Debugf(format, args...)
}

func (l *logger) info(format string, args ...interface{}) {
	format = fmt.Sprintf("[%d] user:%s addr:%s %s", l.id, l.user, l.fromip, format)
	logrus.Infof(format, args...)
}

func (l *logger) err(format string, args ...interface{}) {
	format = fmt.Sprintf("[%d] user:%s addr:%s %s", l.id, l.user, l.fromip, format)
	logrus.Errorf(format, args...)
}
