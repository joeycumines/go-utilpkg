package log

import (
	"github.com/joeycumines/go-utilpkg/sql/log/internal/logrus"
)

type (
	Logrus struct{ logrus.Logger }
)

var (
	_ Logger = Logrus{}
)

func (x Logrus) WithField(key string, value any) Logger {
	return Logrus{Logger: x.Logger.WithField(key, value)}
}

func (x Logrus) WithFields(fields map[string]any) Logger {
	return Logrus{Logger: x.Logger.WithFields(fields)}
}

func (x Logrus) WithError(err error) Logger {
	return Logrus{Logger: x.Logger.WithError(err)}
}
