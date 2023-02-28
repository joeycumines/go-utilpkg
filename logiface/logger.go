package logiface

import (
	"sync"
)

type (
	// Logger is the core logger implementation, and constitutes the core
	// functionality, provided by this package.
	Logger[E Event] struct {
		// WARNING: Fields added must be initialized in both New and Logger.Logger

		level    Level
		modifier Modifier[E]
		shared   *loggerShared[E]
	}

	// loggerShared models the shared state, common between a root Logger
	// instance, and all it's child instances.
	loggerShared[E Event] struct {
		// WARNING: Fields added must be initialized in both New and Logger.Logger

		factory  EventFactory[E]
		releaser EventReleaser[E]
		writer   Writer[E]
		pool     *sync.Pool
	}

	// Option is a configuration option for constructing Logger instances,
	// using the New function, or it's alias(es), e.g. LoggerFactory.New.
	Option[E Event] func(c *loggerConfig[E])

	// loggerConfig is the internal configuration type used by the New function
	loggerConfig[E Event] struct {
		level    Level
		factory  EventFactory[E]
		releaser EventReleaser[E]
		writer   WriterSlice[E]
		modifier ModifierSlice[E]
	}

	// LoggerFactory provides aliases for package functions including New, as
	// well as the functions returning Option values.
	// This allows the Event type to be omitted from all but one location.
	//
	// See also L, an instance of LoggerFactory[Event]{}.
	LoggerFactory[E Event] struct{}
)

var (
	// L exposes New and it's Option functions (e.g. WithWriter) as
	// methods, using the generic Event type.
	//
	// It's provided as a convenience.
	L = LoggerFactory[Event]{}

	// genericBuilderPool is a shared pool for the Builder[Event] type.
	genericBuilderPool = sync.Pool{New: newBuilder[Event]}
)

// WithOptions combines multiple Option values into one.
//
// See also LoggerFactory.WithOptions and L (an instance of LoggerFactory[Event]{}).
func WithOptions[E Event](options ...Option[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		for _, option := range options {
			option(c)
		}
	}
}

// WithOptions is an alias of the package function of the same name.
func (LoggerFactory[E]) WithOptions(options ...Option[E]) Option[E] {
	return WithOptions[E](options...)
}

// WithEventFactory configures the logger's EventFactory.
//
// See also LoggerFactory.WithEventFactory and L (an instance of LoggerFactory[Event]{}).
func WithEventFactory[E Event](factory EventFactory[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.factory = factory
	}
}

// WithEventFactory is an alias of the package function of the same name.
func (LoggerFactory[E]) WithEventFactory(factory EventFactory[E]) Option[E] {
	return WithEventFactory[E](factory)
}

// WithEventReleaser configures the logger's EventReleaser.
//
// See also LoggerFactory.WithEventReleaser and L (an instance of LoggerFactory[Event]{}).
func WithEventReleaser[E Event](releaser EventReleaser[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.releaser = releaser
	}
}

// WithEventReleaser is an alias of the package function of the same name.
func (LoggerFactory[E]) WithEventReleaser(releaser EventReleaser[E]) Option[E] {
	return WithEventReleaser[E](releaser)
}

// WithWriter configures the logger's [Writer], prepending it to an internal
// [WriterSlice].
//
// See also LoggerFactory.WithWriter and L (an instance of LoggerFactory[Event]{}).
func WithWriter[E Event](writer Writer[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		// note: reversed on init
		c.writer = append(c.writer, writer)
	}
}

// WithWriter is an alias of the package function of the same name.
func (LoggerFactory[E]) WithWriter(writer Writer[E]) Option[E] {
	return WithWriter[E](writer)
}

// WithModifier configures the logger's [Modifier], appending it to an internal
// [ModifierSlice].
//
// See also LoggerFactory.WithModifier and L (an instance of LoggerFactory[Event]{}).
func WithModifier[E Event](modifier Modifier[E]) Option[E] {
	return func(c *loggerConfig[E]) {
		c.modifier = append(c.modifier, modifier)
	}
}

// WithModifier is an alias of the package function of the same name.
func (LoggerFactory[E]) WithModifier(modifier Modifier[E]) Option[E] {
	return WithModifier[E](modifier)
}

// WithLevel configures the logger's Level.
//
// Level will be used to filter events that are mapped to a syslog-defined level.
// Events with a custom level will always be logged.
//
// See also LoggerFactory.WithLevel and L (an instance of LoggerFactory[Event]{}).
func WithLevel[E Event](level Level) Option[E] {
	return func(c *loggerConfig[E]) {
		c.level = level
	}
}

// WithLevel is an alias of the package function of the same name.
func (LoggerFactory[E]) WithLevel(level Level) Option[E] {
	return WithLevel[E](level)
}

// New constructs a new Logger instance.
//
// Configure the logger using either the With* prefixed functions (or methods
// of LoggerFactory, e.g. accessible via the L global), or via composite
// options, implemented in external packages, e.g. logger integrations.
//
// See also LoggerFactory.New and L (an instance of LoggerFactory[Event]{}).
func New[E Event](options ...Option[E]) *Logger[E] {
	c := loggerConfig[E]{
		level: LevelInformational,
	}
	for _, o := range options {
		o(&c)
	}

	shared := loggerShared[E]{
		factory:  c.factory,
		releaser: c.releaser,
		writer:   c.resolveWriter(),
	}
	shared.init()

	return &Logger[E]{
		modifier: c.resolveModifier(),
		level:    c.level,
		shared:   &shared,
	}
}

// New is an alias of the package function of the same name.
//
// See also LoggerFactory.New and L (an instance of LoggerFactory[Event]{}).
func (LoggerFactory[E]) New(options ...Option[E]) *Logger[E] {
	return New[E](options...)
}

// Logger returns a new generified logger.
//
// Use this for greater compatibility, but sacrificing ease of using the
// underlying library directly.
func (x *Logger[E]) Logger() *Logger[Event] {
	if x, ok := any(x).(*Logger[Event]); ok {
		return x
	}
	if x == nil || x.shared == nil {
		return nil
	}
	return &Logger[Event]{
		level:    x.level,
		modifier: generifyModifier(x.modifier),
		shared: &loggerShared[Event]{
			factory:  generifyEventFactory(x.shared.factory),
			releaser: generifyEventReleaser(x.shared.releaser),
			writer:   generifyWriter(x.shared.writer),
			pool:     &genericBuilderPool,
		},
	}
}

// Log directly performs a Log operation, without the "fluent builder" pattern.
func (x *Logger[E]) Log(level Level, modifier Modifier[E]) error {
	if !x.canLog(level) {
		return ErrDisabled
	}

	event := x.newEvent(level)
	if x.shared.releaser != nil {
		defer x.shared.releaser.ReleaseEvent(event)
	}

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

// Build returns a new Builder for the given level, note that it may return nil
// (e.g. if the level is disabled).
//
// See also the methods Info, Debug, etc.
func (x *Logger[E]) Build(level Level) *Builder[E] {
	// WARNING must mirror flow of the Log method

	if !x.canLog(level) {
		return nil
	}

	// initialise the builder
	b := x.shared.pool.Get().(*Builder[E])
	b.Event = x.newEvent(level)
	b.shared = x.shared

	// always release b if we don't return it
	var returned bool
	defer func() {
		if !returned {
			b.release()
		}
	}()

	if x.modifier != nil {
		if err := x.modifier.Modify(b.Event); err != nil {
			if err == ErrDisabled {
				return nil
			}
			panic(err)
		}
	}

	returned = true
	return b
}

// Clone returns a new Context, which is a mechanism to configure a sub-logger,
// which will be available via Context.Logger, note that it may return nil.
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
		modifier: ModifierFunc[E](func(event E) error {
			return c.Modifiers.Modify(event)
		}),
		shared: x.shared,
	}

	return &c
}

// Emerg is an alias for Logger.Build(LevelEmergency).
func (x *Logger[E]) Emerg() *Builder[E] { return x.Build(LevelEmergency) }

// Alert is an alias for Logger.Build(LevelAlert).
func (x *Logger[E]) Alert() *Builder[E] { return x.Build(LevelAlert) }

// Crit is an alias for Logger.Build(LevelCritical).
func (x *Logger[E]) Crit() *Builder[E] { return x.Build(LevelCritical) }

// Err is an alias for Logger.Build(LevelError).
func (x *Logger[E]) Err() *Builder[E] { return x.Build(LevelError) }

// Warning is an alias for Logger.Build(LevelWarning).
func (x *Logger[E]) Warning() *Builder[E] { return x.Build(LevelWarning) }

// Notice is an alias for Logger.Build(LevelNotice).
func (x *Logger[E]) Notice() *Builder[E] { return x.Build(LevelNotice) }

// Info is an alias for Logger.Build(LevelInformational).
func (x *Logger[E]) Info() *Builder[E] { return x.Build(LevelInformational) }

// Debug is an alias for Logger.Build(LevelDebug).
func (x *Logger[E]) Debug() *Builder[E] { return x.Build(LevelDebug) }

// Trace is an alias for Logger.Build(LevelTrace).
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

func (x *loggerShared[E]) init() {
	switch any(x).(type) {
	case *loggerShared[Event]:
		x.pool = &genericBuilderPool
	default:
		x.pool = &sync.Pool{New: newBuilder[E]}
	}
}

func (x *loggerConfig[E]) resolveWriter() Writer[E] {
	switch len(x.writer) {
	case 0:
		return nil
	case 1:
		return x.writer[0]
	default:
		reverseSlice(x.writer)
		return x.writer
	}
}

func (x *loggerConfig[E]) resolveModifier() Modifier[E] {
	switch len(x.modifier) {
	case 0:
		return nil
	case 1:
		return x.modifier[0]
	default:
		return x.modifier
	}
}

func reverseSlice[S ~[]E, E any](s S) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func generifyModifier[E Event](modifier Modifier[E]) Modifier[Event] {
	if modifier == nil {
		return nil
	}
	return ModifierFunc[Event](func(event Event) error {
		return modifier.Modify(event.(E))
	})
}

func generifyWriter[E Event](writer Writer[E]) Writer[Event] {
	if writer == nil {
		return nil
	}
	return WriterFunc[Event](func(event Event) error {
		return writer.Write(event.(E))
	})
}

func generifyEventFactory[E Event](factory EventFactory[E]) EventFactory[Event] {
	if factory == nil {
		return nil
	}
	return EventFactoryFunc[Event](func(level Level) Event {
		return factory.NewEvent(level)
	})
}

func generifyEventReleaser[E Event](releaser EventReleaser[E]) EventReleaser[Event] {
	if releaser == nil {
		return nil
	}
	return EventReleaserFunc[Event](func(event Event) {
		releaser.ReleaseEvent(event.(E))
	})
}

func newBuilder[E Event]() any {
	return new(Builder[E])
}
