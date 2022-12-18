package logiface

import (
	"fmt"
)

type (
	// Context is used to build a sub-logger, see Logger.With.
	Context[E Event] struct {
		Modifiers ModifierSlice[E]
		modifierMethods[*Context[E], E]
		logger *Logger[E]
	}

	// Builder is used to build a log event, see Logger.Build, Logger.Info, etc.
	Builder[E Event] struct {
		Event E
		modifierMethods[*Builder[E], E]
		logger *Logger[E]
	}

	modifierMethods[R any, E Event] struct {
		res R
		fn  func(modifier Modifier[E])
	}
)

func (x *Context[E]) Logger() *Logger[E] {
	return x.logger
}

func (x *Builder[E]) Call(fn func(b *Builder[E])) *Builder[E] {
	fn(x)
	return x
}

func (x *Builder[E]) Log(msg string) {
	if x.Event.Level().Enabled() {
		x.log(msg)
	}
}

func (x *Builder[E]) Logf(format string, args ...any) {
	if x.Event.Level().Enabled() {
		x.log(fmt.Sprintf(format, args...))
	}
}

func (x *Builder[E]) LogFunc(fn func() string) {
	if x.Event.Level().Enabled() {
		x.log(fn())
	}
}

func (x *Builder[E]) log(msg string) {
	x.Event.SetMessage(msg)
	_ = x.logger.writer.Write(x.Event)
}

func (x modifierMethods[R, E]) With(key string, val any) R {
	x.fn(ModifyFunc[E](func(event E) error {
		if event.Level().Enabled() {
			event.AddField(key, val)
		}
		return nil
	}))
	return x.res
}
