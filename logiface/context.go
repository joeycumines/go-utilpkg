package logiface

import (
	"fmt"
)

type (
	// Context is used to build a sub-logger, see Logger.With.
	Context[E Event] struct {
		Modifiers ModifierSlice[E]
		methods   modifierMethods[E]
		logger    *Logger[E]
	}

	// Builder is used to build a log event, see Logger.Build, Logger.Info, etc.
	Builder[E Event] struct {
		Event   E
		methods modifierMethods[E]
		shared  *loggerShared[E]
	}

	modifierMethods[E Event] func(modifier Modifier[E])
)

func (x *Context[E]) Logger() *Logger[E] {
	if x == nil {
		return nil
	}
	return x.logger
}

func (x *Builder[E]) Call(fn func(b *Builder[E])) *Builder[E] {
	fn(x)
	return x
}

func (x *Builder[E]) Log(msg string) {
	if x == nil {
		return
	}
	defer x.release()
	if x.Event.Level().Enabled() {
		x.log(msg)
	}
}

func (x *Builder[E]) Logf(format string, args ...any) {
	if x == nil {
		return
	}
	defer x.release()
	if x.Event.Level().Enabled() {
		x.log(fmt.Sprintf(format, args...))
	}
}

func (x *Builder[E]) LogFunc(fn func() string) {
	if x == nil {
		return
	}
	defer x.release()
	if x.Event.Level().Enabled() {
		x.log(fn())
	}
}

func (x *Builder[E]) log(msg string) {
	x.Event.SetMessage(msg)
	_ = x.shared.writer.Write(x.Event)
}

func (x *Builder[E]) release() {
	if x.shared != nil {
		x.shared.pool.Put(x)
	}
}

func (x modifierMethods[E]) With(key string, val any) {
	x(ModifyFunc[E](func(event E) error {
		if event.Level().Enabled() {
			event.AddField(key, val)
		}
		return nil
	}))
}
func (x *Context[E]) With(key string, val any) *Context[E] {
	if x != nil && x.methods != nil {
		x.methods.With(key, val)
	}
	return x
}
func (x *Builder[E]) With(key string, val any) *Builder[E] {
	if x != nil && x.methods != nil {
		x.methods.With(key, val)
	}
	return x
}
