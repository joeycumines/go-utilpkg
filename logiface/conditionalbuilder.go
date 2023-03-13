package logiface

import (
	"encoding/base64"
	"time"
)

type (
	// ConditionalBuilder models a subset of [Builder] that is either enabled
	// or disabled, e.g. depending on the configured log level.
	//
	// See also [Builder.IfTrace].
	ConditionalBuilder[E Event] interface {
		// Builder returns the underlying [Builder], or nil.
		Builder() *Builder[E]

		// Enabled returns true if the receiver is enabled.
		// Note that it _may_ be false even if the corresponding
		//[Builder.Enabled] value is true, but not the other way around.
		Enabled() bool

		// Else converts a disabled builder to an enabled builder, an enabled
		// builder to a terminated builder (disabled until unwrapped via
		// [ConditionalBuilder.Builder]), or returns the same terminated
		// builder.
		Else() ConditionalBuilder[E]

		// ElseIf will either return a terminated builder, in the same cases as
		// [ConditionalBuilder.Else], or will return either an enabled or
		// disabled builder, where cond of true is necessary to enable the
		// builder.
		//
		// See also [Builder.If].
		ElseIf(cond bool) ConditionalBuilder[E]

		// ElseIfFunc will either return a terminated builder, in the same cases
		// as [ConditionalBuilder.Else], or will return either an enabled or
		// disabled builder, where the condition evaluating as true will enable
		// the builder.
		//
		// See also [Builder.IfFunc], for details on how the condition is
		// evaluated.
		ElseIfFunc(cond func() bool) ConditionalBuilder[E]

		// ElseIfLevel will either return a terminated builder, in the same cases
		// as [ConditionalBuilder.Else], or will return either an enabled or
		// disabled builder, where the condition evaluating as true will enable
		// the builder.
		//
		// See also [Builder.IfLevel], for details on how the condition is
		// evaluated.
		ElseIfLevel(level Level) ConditionalBuilder[E]

		// ElseIfEmerg is an alias for [ConditionalBuilder.ElseIfLevel]([LevelEmergency]).
		ElseIfEmerg() ConditionalBuilder[E]

		// ElseIfAlert is an alias for [ConditionalBuilder.ElseIfLevel]([LevelAlert]).
		ElseIfAlert() ConditionalBuilder[E]

		// ElseIfCrit is an alias for [ConditionalBuilder.ElseIfLevel]([LevelCritical]).
		ElseIfCrit() ConditionalBuilder[E]

		// ElseIfErr is an alias for [ConditionalBuilder.ElseIfLevel]([LevelError]).
		ElseIfErr() ConditionalBuilder[E]

		// ElseIfWarning is an alias for [ConditionalBuilder.ElseIfLevel]([LevelWarning]).
		ElseIfWarning() ConditionalBuilder[E]

		// ElseIfNotice is an alias for [ConditionalBuilder.ElseIfLevel]([LevelNotice]).
		ElseIfNotice() ConditionalBuilder[E]

		// ElseIfInfo is an alias for [ConditionalBuilder.ElseIfLevel]([LevelInformational]).
		ElseIfInfo() ConditionalBuilder[E]

		// ElseIfDebug is an alias for [ConditionalBuilder.ElseIfLevel]([LevelDebug]).
		ElseIfDebug() ConditionalBuilder[E]

		// ElseIfTrace is an alias for [ConditionalBuilder.ElseIfLevel]([LevelTrace]).
		ElseIfTrace() ConditionalBuilder[E]

		// Call performs [Builder.Call] conditionally.
		Call(fn func(b *Builder[E])) ConditionalBuilder[E]

		// Field performs [Builder.Field] conditionally.
		Field(key string, val any) ConditionalBuilder[E]

		// Any performs [Builder.Any] conditionally.
		Any(key string, val any) ConditionalBuilder[E]

		// Base64 performs [Builder.Base64] conditionally.
		Base64(key string, b []byte, enc *base64.Encoding) ConditionalBuilder[E]

		// Dur performs [Builder.Dur] conditionally.
		Dur(key string, d time.Duration) ConditionalBuilder[E]

		// Err performs [Builder.Err] conditionally.
		Err(err error) ConditionalBuilder[E]

		// Float32 performs [Builder.Float32] conditionally.
		Float32(key string, val float32) ConditionalBuilder[E]

		// Int performs [Builder.Int] conditionally.
		Int(key string, val int) ConditionalBuilder[E]

		// Interface performs [Builder.Interface] conditionally.
		Interface(key string, val any) ConditionalBuilder[E]

		// Str performs [Builder.Str] conditionally.
		Str(key string, val string) ConditionalBuilder[E]

		// Time performs [Builder.Time] conditionally.
		Time(key string, t time.Time) ConditionalBuilder[E]

		// Bool performs [Builder.Bool] conditionally.
		Bool(key string, val bool) ConditionalBuilder[E]

		// Float64 performs [Builder.Float64] conditionally.
		Float64(key string, val float64) ConditionalBuilder[E]

		// Int64 performs [Builder.Int64] conditionally.
		Int64(key string, val int64) ConditionalBuilder[E]

		// Uint64 performs [Builder.Uint64] conditionally.
		Uint64(key string, val uint64) ConditionalBuilder[E]

		// future additions are expected + implemented within this pkg
		isConditionalBuilder()
	}

	enabledBuilder[E Event] Builder[E]

	disabledBuilder[E Event] Builder[E]

	terminatedBuilder[E Event] Builder[E]
)

var (
	// compile time assertions

	_ ConditionalBuilder[Event] = (*enabledBuilder[Event])(nil)
	_ ConditionalBuilder[Event] = (*disabledBuilder[Event])(nil)
	_ ConditionalBuilder[Event] = (*terminatedBuilder[Event])(nil)
)

// If converts the receiver into a [ConditionalBuilder], which exposes the same
// set of non-terminating methods as [Builder], guarded such that they do not
// log unless the given condition is true.
func (x *Builder[E]) If(cond bool) ConditionalBuilder[E] {
	if cond && x.Enabled() {
		return (*enabledBuilder[E])(x)
	}
	return (*disabledBuilder[E])(x)
}

// IfFunc converts the receiver into a [ConditionalBuilder], which exposes the
// same set of non-terminating methods as [Builder], guarded such that they do
// not log unless the given condition (modeled as a function) is true.
//
// A nil cond func wil be treated as disabled.
//
// This method differs from [Builder.If] in that the condition is evaluated
// only if the receiver is enabled, see also [Builder.Enabled].
func (x *Builder[E]) IfFunc(cond func() bool) ConditionalBuilder[E] {
	if cond != nil && x.Enabled() && cond() {
		return (*enabledBuilder[E])(x)
	}
	return (*disabledBuilder[E])(x)
}

// IfLevel converts the receiver into a [ConditionalBuilder], which exposes the
// same set of non-terminating methods as [Builder], guarded such that they do
// not log unless the [Logger] level is >= the given level.
//
// See also [Level].
func (x *Builder[E]) IfLevel(level Level) ConditionalBuilder[E] {
	if x.Enabled() && x.shared.level >= level {
		return (*enabledBuilder[E])(x)
	}
	return (*disabledBuilder[E])(x)
}

// IfEmerg is an alias for [Builder.IfLevel]([LevelEmergency]).
func (x *Builder[E]) IfEmerg() ConditionalBuilder[E] { return x.IfLevel(LevelEmergency) }

// IfAlert is an alias for [Builder.IfLevel]([LevelAlert]).
func (x *Builder[E]) IfAlert() ConditionalBuilder[E] { return x.IfLevel(LevelAlert) }

// IfCrit is an alias for [Builder.IfLevel]([LevelCritical]).
func (x *Builder[E]) IfCrit() ConditionalBuilder[E] { return x.IfLevel(LevelCritical) }

// IfErr is an alias for [Builder.IfLevel]([LevelError]).
func (x *Builder[E]) IfErr() ConditionalBuilder[E] { return x.IfLevel(LevelError) }

// IfWarning is an alias for [Builder.IfLevel]([LevelWarning]).
func (x *Builder[E]) IfWarning() ConditionalBuilder[E] { return x.IfLevel(LevelWarning) }

// IfNotice is an alias for [Builder.IfLevel]([LevelNotice]).
func (x *Builder[E]) IfNotice() ConditionalBuilder[E] { return x.IfLevel(LevelNotice) }

// IfInfo is an alias for [Builder.IfLevel]([LevelInformational]).
func (x *Builder[E]) IfInfo() ConditionalBuilder[E] { return x.IfLevel(LevelInformational) }

// IfDebug is an alias for [Builder.IfLevel]([LevelDebug]).
func (x *Builder[E]) IfDebug() ConditionalBuilder[E] { return x.IfLevel(LevelDebug) }

// IfTrace is an alias for [Builder.IfLevel]([LevelTrace]).
func (x *Builder[E]) IfTrace() ConditionalBuilder[E] { return x.IfLevel(LevelTrace) }

func (x *enabledBuilder[E]) Builder() *Builder[E] { return (*Builder[E])(x) }

func (x *enabledBuilder[E]) Enabled() bool { return true }

func (x *enabledBuilder[E]) Else() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIf(cond bool) ConditionalBuilder[E] {
	return (*terminatedBuilder[E])(x)
}

func (x *enabledBuilder[E]) ElseIfFunc(cond func() bool) ConditionalBuilder[E] {
	return (*terminatedBuilder[E])(x)
}

func (x *enabledBuilder[E]) ElseIfLevel(level Level) ConditionalBuilder[E] {
	return (*terminatedBuilder[E])(x)
}

func (x *enabledBuilder[E]) ElseIfEmerg() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfAlert() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfCrit() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfErr() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfWarning() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfNotice() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfInfo() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfDebug() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) ElseIfTrace() ConditionalBuilder[E] { return (*terminatedBuilder[E])(x) }

func (x *enabledBuilder[E]) Call(fn func(b *Builder[E])) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Call(fn))
}

func (x *enabledBuilder[E]) Field(key string, val any) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Field(key, val))
}

func (x *enabledBuilder[E]) Any(key string, val any) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Any(key, val))
}

func (x *enabledBuilder[E]) Base64(key string, b []byte, enc *base64.Encoding) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Base64(key, b, enc))
}

func (x *enabledBuilder[E]) Dur(key string, d time.Duration) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Dur(key, d))
}

func (x *enabledBuilder[E]) Err(err error) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Err(err))
}

func (x *enabledBuilder[E]) Float32(key string, val float32) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Float32(key, val))
}

func (x *enabledBuilder[E]) Int(key string, val int) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Int(key, val))
}

func (x *enabledBuilder[E]) Interface(key string, val any) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Interface(key, val))
}

func (x *enabledBuilder[E]) Str(key string, val string) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Str(key, val))
}

func (x *enabledBuilder[E]) Time(key string, t time.Time) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Time(key, t))
}

func (x *enabledBuilder[E]) Bool(key string, val bool) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Bool(key, val))
}

func (x *enabledBuilder[E]) Float64(key string, val float64) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Float64(key, val))
}

func (x *enabledBuilder[E]) Int64(key string, val int64) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Int64(key, val))
}

func (x *enabledBuilder[E]) Uint64(key string, val uint64) ConditionalBuilder[E] {
	return (*enabledBuilder[E])((*Builder[E])(x).Uint64(key, val))
}

//lint:ignore U1000 implements interface
func (x *enabledBuilder[E]) isConditionalBuilder() {}

func (x *disabledBuilder[E]) Builder() *Builder[E] { return (*Builder[E])(x) }

func (x *disabledBuilder[E]) Enabled() bool { return false }

func (x *disabledBuilder[E]) Else() ConditionalBuilder[E] { return (*enabledBuilder[E])(x) }

func (x *disabledBuilder[E]) ElseIf(cond bool) ConditionalBuilder[E] {
	return (*Builder[E])(x).If(cond)
}

func (x *disabledBuilder[E]) ElseIfFunc(cond func() bool) ConditionalBuilder[E] {
	return (*Builder[E])(x).IfFunc(cond)
}

func (x *disabledBuilder[E]) ElseIfLevel(level Level) ConditionalBuilder[E] {
	return (*Builder[E])(x).IfLevel(level)
}

func (x *disabledBuilder[E]) ElseIfEmerg() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfEmerg()
}

func (x *disabledBuilder[E]) ElseIfAlert() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfAlert()
}

func (x *disabledBuilder[E]) ElseIfCrit() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfCrit()
}

func (x *disabledBuilder[E]) ElseIfErr() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfErr()
}

func (x *disabledBuilder[E]) ElseIfWarning() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfWarning()
}

func (x *disabledBuilder[E]) ElseIfNotice() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfNotice()
}

func (x *disabledBuilder[E]) ElseIfInfo() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfInfo()
}

func (x *disabledBuilder[E]) ElseIfDebug() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfDebug()
}

func (x *disabledBuilder[E]) ElseIfTrace() ConditionalBuilder[E] {
	return (*Builder[E])(x).IfTrace()
}

func (x *disabledBuilder[E]) Call(func(b *Builder[E])) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Field(string, any) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Any(key string, val any) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Base64(key string, b []byte, enc *base64.Encoding) ConditionalBuilder[E] {
	return x
}

func (x *disabledBuilder[E]) Dur(key string, d time.Duration) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Err(err error) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Float32(key string, val float32) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Int(key string, val int) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Interface(key string, val any) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Str(key string, val string) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Time(key string, t time.Time) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Bool(key string, val bool) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Float64(key string, val float64) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Int64(key string, val int64) ConditionalBuilder[E] { return x }

func (x *disabledBuilder[E]) Uint64(key string, val uint64) ConditionalBuilder[E] { return x }

//lint:ignore U1000 implements interface
func (x *disabledBuilder[E]) isConditionalBuilder() {}

func (x *terminatedBuilder[E]) Builder() *Builder[E] { return (*Builder[E])(x) }

func (x *terminatedBuilder[E]) Enabled() bool { return false }

func (x *terminatedBuilder[E]) Else() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIf(cond bool) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfFunc(cond func() bool) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfLevel(level Level) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfEmerg() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfAlert() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfCrit() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfErr() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfWarning() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfNotice() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfInfo() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfDebug() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) ElseIfTrace() ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Call(func(b *Builder[E])) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Field(string, any) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Any(key string, val any) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Base64(key string, b []byte, enc *base64.Encoding) ConditionalBuilder[E] {
	return x
}

func (x *terminatedBuilder[E]) Dur(key string, d time.Duration) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Err(err error) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Float32(key string, val float32) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Int(key string, val int) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Interface(key string, val any) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Str(key string, val string) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Time(key string, t time.Time) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Bool(key string, val bool) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Float64(key string, val float64) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Int64(key string, val int64) ConditionalBuilder[E] { return x }

func (x *terminatedBuilder[E]) Uint64(key string, val uint64) ConditionalBuilder[E] { return x }

//lint:ignore U1000 implements interface
func (x *terminatedBuilder[E]) isConditionalBuilder() {}
