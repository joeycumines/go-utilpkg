package logiface

import (
	"sync"
)

type (
	Logger[E Event] struct {
		level    Level
		modifier Modifier[E]
		shared   *loggerShared[E]
	}

	loggerShared[E Event] struct {
		factory EventFactory[E]
		writer  Writer[E]
		pool    *sync.Pool
	}

	Option[E Event] func(c *loggerConfig[E])

	loggerConfig[E Event] struct {
		level    Level
		factory  EventFactory[E]
		writer   Writer[E]
		modifier Modifier[E]
	}
)

func WithLogger[E Event](impl LoggerImpl[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.factory = impl
		c.writer = impl
	}
}

func WithEventFactory[E Event](factory EventFactory[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.factory = factory
	}
}

func WithWriter[E Event](writer Writer[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.writer = writer
	}
}

func WithModifier[E Event](modifier Modifier[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.modifier = modifier
	}
}

func WithLevel[E Event](level Level) Option[E] {
	return func(c *loggerConfig[E]) {
		c.level = level
	}
}

func New[E Event](options ...Option[E]) *Logger[E] {
	c := loggerConfig[E]{
		level: LevelInformational,
	}
	for _, o := range options {
		o(&c)
	}

	shared := loggerShared[E]{
		factory: c.factory,
		writer:  c.writer,
	}
	shared.pool = &sync.Pool{New: shared.newBuilder}

	return &Logger[E]{
		modifier: c.modifier,
		level:    c.level,
		shared:   &shared,
	}
}

// Logger returns a new generified logger.
// Use this for greater compatibility, but sacrificing ease of using the
// underlying library directly.
func (x *Logger[E]) Logger() *Logger[Event] {
	if x, ok := any(x).(*Logger[Event]); ok {
		return x
	}
	// TODO implement wrappers for EventFactory and Writer
	panic(`not implemented`)
}

func (x *Logger[E]) Log(level Level, modifier Modifier[E]) error {
	if !x.canLog(level) {
		return ErrDisabled
	}

	event := x.newEvent(level)

	if x.modifier != nil {
		if err := x.modifier.Modify(event); err != nil {
			return err
		}
	}

	if modifier != nil {
		if err := modifier.Modify(event); err != nil {
			return err
		}
	}

	return x.shared.writer.Write(event)
}

func (x *Logger[E]) Build(level Level) *Builder[E] {
	// WARNING must mirror flow of the Log method

	if !x.canLog(level) {
		return nil
	}

	b := x.shared.pool.Get().(*Builder[E])
	b.Event = x.newEvent(level)

	if x.modifier != nil {
		if err := x.modifier.Modify(b.Event); err != nil {
			if err == ErrDisabled {
				return nil
			}
			panic(err)
		}
	}

	return b
}

func (x *Logger[E]) Clone() *Context[E] {
	if !x.canWrite() {
		return nil
	}

	var c Context[E]
	if x.modifier != nil {
		c.Modifiers = append(c.Modifiers, x.modifier)
	}
	c.logger = &Logger[E]{
		level: x.level,
		modifier: ModifyFunc[E](func(event E) error {
			return c.Modifiers.Modify(event)
		}),
		shared: x.shared,
	}

	return &c
}

func (x *Logger[E]) Emerg() *Builder[E] { return x.Build(LevelEmergency) }

func (x *Logger[E]) Alert() *Builder[E] { return x.Build(LevelAlert) }

func (x *Logger[E]) Crit() *Builder[E] { return x.Build(LevelCritical) }

func (x *Logger[E]) Err() *Builder[E] { return x.Build(LevelError) }

func (x *Logger[E]) Warning() *Builder[E] { return x.Build(LevelWarning) }

func (x *Logger[E]) Notice() *Builder[E] { return x.Build(LevelNotice) }

func (x *Logger[E]) Info() *Builder[E] { return x.Build(LevelInformational) }

func (x *Logger[E]) Debug() *Builder[E] { return x.Build(LevelDebug) }

func (x *Logger[E]) Trace() *Builder[E] { return x.Build(LevelTrace) }

func (x *Logger[E]) canWrite() bool {
	return x != nil &&
		x.shared != nil &&
		x.shared.writer != nil
}

func (x *Logger[E]) canLog(level Level) bool {
	return x.canWrite() &&
		level.Enabled() &&
		(level <= x.level || level > LevelTrace)
}

func (x *Logger[E]) newEvent(level Level) (event E) {
	if x != nil && x.shared != nil && x.shared.factory != nil {
		event = x.shared.factory.NewEvent(level)
	}
	return
}

func (x *loggerShared[E]) newBuilder() any {
	return &Builder[E]{shared: x}
}
