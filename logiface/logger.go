package logiface

type (
	Logger[E Event] struct {
		factory         EventFactory[E]
		writer          Writer[E]
		modifier        Modifier[E]
		level           Level
		disabledBuilder *Builder[E]
	}

	Option[E Event] func(c *loggerConfig[E])

	loggerConfig[E Event] struct {
		factory  EventFactory[E]
		writer   Writer[E]
		modifier Modifier[E]
		level    Level
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

	x := Logger[E]{
		factory:  c.factory,
		writer:   c.writer,
		modifier: c.modifier,
		level:    c.level,
	}

	x.initDisabledBuilder()

	return &x
}

func (x *Logger[E]) Logger() *Logger[E] { return x }

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

	return x.writer.Write(event)
}

func (x *Logger[E]) Build(level Level) *Builder[E] {
	// WARNING must mirror flow of the Log method

	if !x.canLog(level) {
		if x == nil {
			return x.newDisabledBuilder()
		}
		return x.disabledBuilder
	}

	b := Builder[E]{
		Event:  x.newEvent(level),
		logger: x,
	}

	if x.modifier != nil {
		if err := x.modifier.Modify(b.Event); err != nil {
			if err == ErrDisabled {
				return x.disabledBuilder
			}
			panic(err)
		}
	}

	b.modifierMethods = modifierMethods[*Builder[E], E]{
		res: &b,
		fn: func(modifier Modifier[E]) {
			_ = modifier.Modify(b.Event)
		},
	}

	return &b
}

func (x *Logger[E]) Clone() *Context[E] {
	if !x.canWrite() {
		c := Context[E]{logger: new(Logger[E])}
		c.modifierMethods = modifierMethods[*Context[E], E]{
			res: &c,
			fn:  func(modifier Modifier[E]) {},
		}
		if x != nil && x.disabledBuilder != nil {
			c.logger.disabledBuilder = x.disabledBuilder
		} else {
			c.logger.initDisabledBuilder()
		}
		return &c
	}

	var c Context[E]

	if x.modifier != nil {
		c.Modifiers = append(c.Modifiers, x.modifier)
	}

	c.modifierMethods = modifierMethods[*Context[E], E]{
		res: &c,
		fn: func(modifier Modifier[E]) {
			c.Modifiers = append(c.Modifiers, modifier)
		},
	}

	c.logger = &Logger[E]{
		factory: x.factory,
		writer:  x.writer,
		level:   x.level,
		modifier: ModifyFunc[E](func(event E) error {
			return c.Modifiers.Modify(event)
		}),
	}

	if x.disabledBuilder != nil {
		c.logger.disabledBuilder = x.disabledBuilder
	} else {
		c.logger.initDisabledBuilder()
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
		x.writer != nil
}

func (x *Logger[E]) canLog(level Level) bool {
	return x.canWrite() &&
		level.Enabled() &&
		level <= x.level
}

func (x *Logger[E]) newEvent(level Level) (event E) {
	if x != nil && x.factory != nil {
		event = x.factory.NewEvent(level)
	}
	return
}

func (x *Logger[E]) newDisabledBuilder() (b *Builder[E]) {
	b = new(Builder[E])
	b.modifierMethods = modifierMethods[*Builder[E], E]{
		res: b,
		fn:  func(modifier Modifier[E]) {},
	}
	return
}

func (x *Logger[E]) initDisabledBuilder() {
	x.disabledBuilder = x.newDisabledBuilder()
}
