// Package logrus only exists to give the name Logger to logrus.FieldLogger, for embedding.
package logrus

import (
	"github.com/sirupsen/logrus"
)

type (
	// Logger is logrus.FieldLogger.
	Logger = logrus.FieldLogger
)
