package stumpy

import (
	"encoding/json"
	"github.com/joeycumines/logiface"
	"io"
	"os"
)

type (
	// LoggerFactory is provided as a convenience, embedding
	// logiface.LoggerFactory[*Event], and aliasing the (logiface) option
	// functions implemented within this package.
	LoggerFactory struct {
		//lint:ignore U1000 embedded for it's methods
		baseLoggerFactory
	}

	//lint:ignore U1000 used to embed without exporting
	baseLoggerFactory = logiface.LoggerFactory[*Event]

	// Option models a configuration option for this package's logger, see also
	// the package level functions, returning values of this type.
	Option interface {
		apply(c *loggerConfig)
	}

	optionFunc func(c *loggerConfig)

	loggerConfig struct {
		writer       io.Writer
		timeField    *string
		levelField   *string
		messageField *string
		errorField   *string
	}
)

var (
	// compile time assertions

	_ Option = optionFunc(nil)
)

var (
	// L is a LoggerFactory, and may be used to configure a
	// logiface.Logger[*Event], using the implementations provided by this
	// package.
	L = LoggerFactory{}
)

func (x optionFunc) apply(c *loggerConfig) { x(c) }

// WithStumpy configures a logiface logger to use a stumpy logger.
func WithStumpy(options ...Option) logiface.Option[*Event] {
	var c loggerConfig
	for _, o := range options {
		o.apply(&c)
	}

	l := Logger{}

	if c.writer == nil {
		l.writer = os.Stderr
	} else {
		l.writer = c.writer
	}

	if c.timeField != nil && *c.timeField != `` {
		b, err := json.Marshal(*c.timeField)
		if err != nil {
			panic(err)
		}
		l.timeField = string(b)
	}

	if c.levelField == nil {
		l.levelField = `"lvl"`
	} else if *c.levelField != `` {
		b, err := json.Marshal(*c.levelField)
		if err != nil {
			panic(err)
		}
		l.levelField = string(b)
	}

	if c.messageField == nil || *c.messageField == `` {
		l.messageField = `"msg"`
	} else {
		b, err := json.Marshal(*c.messageField)
		if err != nil {
			panic(err)
		}
		l.messageField = string(b)
	}

	if c.errorField == nil || *c.errorField == `` {
		l.errorField = `"err"`
	} else {
		b, err := json.Marshal(*c.errorField)
		if err != nil {
			panic(err)
		}
		l.errorField = string(b)
	}

	return L.WithOptions(
		L.WithWriter(&l),
		L.WithEventFactory(&l),
		L.WithEventReleaser(&l),
		logiface.WithJSONSupport[*Event, *Event, *Event](&l),
	)
}

// WithStumpy is an alias of the package function of the same name.
func (LoggerFactory) WithStumpy(options ...Option) logiface.Option[*Event] {
	return WithStumpy(options...)
}

func WithWriter(writer io.Writer) Option {
	return optionFunc(func(c *loggerConfig) {
		c.writer = writer
	})
}

func WithTimeField(field string) Option {
	return optionFunc(func(c *loggerConfig) {
		c.timeField = &field
	})
}

func WithLevelField(field string) Option {
	return optionFunc(func(c *loggerConfig) {
		c.levelField = &field
	})
}

func WithMessageField(field string) Option {
	return optionFunc(func(c *loggerConfig) {
		c.messageField = &field
	})
}

func WithErrorField(field string) Option {
	return optionFunc(func(c *loggerConfig) {
		c.errorField = &field
	})
}
