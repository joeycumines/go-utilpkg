package zerolog

import (
	"github.com/rs/zerolog"
)

type (
	logObjectMarshaler func(e *zerolog.Event)
)

func (x logObjectMarshaler) MarshalZerologObject(e *zerolog.Event) { x(e) }

func (x logObjectMarshaler) Error() string { return `logObjectMarshaler` }
