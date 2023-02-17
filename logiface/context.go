package logiface

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type (
	// Context is used to build a sub-logger, see Logger.Clone.
	//
	// All methods are safe to call on a nil receiver.
	//
	// See also Builder, which implements a common (sub)set of methods, for
	// building structured log events, including Field ("smarter" than
	// Interface), Interface (Event.AddField pass-through), and strongly-typed
	// implementations that utilize the other Event.Add* methods (if
	// implemented), with well-defined fallback behavior.
	Context[E Event] struct {
		Modifiers ModifierSlice[E]
		methods   modifierMethods[E]
		logger    *Logger[E]
	}

	// Builder is used to build a log event, see Logger.Build, Logger.Info, etc.
	//
	// All methods are safe to call on a nil receiver.
	//
	// See also Context, which implements a common (sub)set of methods, for
	// building structured log events, including Field ("smarter" than
	// Interface), Interface (Event.AddField pass-through), and strongly-typed
	// implementations such as Err and Str, that utilize the other Event.Add*
	// methods (if implemented), with well-defined fallback behavior.
	Builder[E Event] struct {
		Event E

		// WARNING: If additional fields are added, they may need to be released
		// (see Builder.release)

		methods modifierMethods[E]
		shared  *loggerShared[E]
	}

	modifierMethods[E Event] struct{}
)

// Logger returns the underlying (sub)logger, or nil.
//
// Note that the returned logger will apply all parent modifiers, including
// Context.Modifiers. This method is intended to be used to get the actual
// sub-logger, after building the context that sub-logger is to apply.
//
// This method is not implemented by Builder.
func (x *Context[E]) Logger() *Logger[E] {
	if x == nil {
		return nil
	}
	return x.logger
}

func (x *Context[E]) add(fn ModifierFunc[E]) {
	x.Modifiers = append(x.Modifiers, fn)
}

// ok returns true if the receiver is initialized
func (x *Context[E]) ok() bool {
	return x != nil && x.logger != nil
}

// Call is provided as a convenience, to facilitate code which uses the
// receiver explicitly, without breaking out of the fluent-style API.
//
// Example use cases include access of the underlying Event implementation (to
// utilize functionality not mapped by logiface), or to skip building /
// formatting certain fields, based on if `b.Event.Level().Enabled()`.
//
// This method is not implemented by Context.
func (x *Builder[E]) Call(fn func(b *Builder[E])) *Builder[E] {
	fn(x)
	return x
}

// Log logs an event, with the given message, using the provided level and
// modifier, if the relevant conditions are met (e.g. configured log level).
//
// The field for the message will either be determined by the implementation,
// if Event.AddMessage is implemented, or the default field name "msg" will be
// used.
//
// This method is not implemented by Context.
func (x *Builder[E]) Log(msg string) {
	if !x.ok() {
		return
	}
	defer x.release()
	if x.Event.Level().Enabled() {
		x.log(msg)
	}
}

// Logf logs an event, with the given message, using the provided level and
// modifier, if the relevant conditions are met (e.g. configured log level).
//
// The field for the message will either be determined by the implementation,
// if Event.AddMessage is implemented, or the default field name "msg" will be
// used.
//
// This method is not implemented by Context.
func (x *Builder[E]) Logf(format string, args ...any) {
	if !x.ok() {
		return
	}
	defer x.release()
	if x.Event.Level().Enabled() {
		x.log(fmt.Sprintf(format, args...))
	}
}

// LogFunc logs an event, with the message returned by the given function,
// using the provided level and modifier, if the relevant conditions are met
// (e.g. configured log level).
//
// The field for the message will either be determined by the implementation,
// if Event.AddMessage is implemented, or the default field name "msg" will be
// used.
//
// Note that the function will not be called if, for example, the given log
// level is not enabled.
//
// This method is not implemented by Context.
func (x *Builder[E]) LogFunc(fn func() string) {
	if !x.ok() {
		return
	}
	defer x.release()
	if x.Event.Level().Enabled() {
		x.log(fn())
	}
}

func (x *Builder[E]) log(msg string) {
	if msg != `` && !x.Event.AddMessage(msg) {
		x.Event.AddField(`msg`, msg)
	}
	_ = x.shared.writer.Write(x.Event)
}

func (x *Builder[E]) release() {
	if shared := x.shared; shared != nil {
		x.shared = nil
		if shared.releaser != nil {
			shared.releaser.ReleaseEvent(x.Event)
		}
		shared.pool.Put(x)
	}
}

func (x *Builder[E]) ok() bool {
	return x != nil && x.shared != nil
}

func (x modifierMethods[E]) str(event E, key string, val string) {
	if !event.AddString(key, val) {
		event.AddField(key, val)
	}
}

func (x modifierMethods[E]) bytes(event E, key string, val []byte) {
	// TODO allow custom handling via an optional method
	x.str(event, key, base64.StdEncoding.EncodeToString(val))
}

func (x modifierMethods[E]) time(event E, key string, val time.Time) {
	if !event.AddTime(key, val) {
		x.str(event, key, formatTimestamp(val))
	}
}

func (x modifierMethods[E]) duration(event E, key string, val time.Duration) {
	// TODO allow custom handling via an optional method
	x.str(event, key, formatDuration(val))
}

func (x modifierMethods[E]) int(event E, key string, val int) {
	if !event.AddInt(key, val) {
		event.AddField(key, val)
	}
}

func (x modifierMethods[E]) float32(event E, key string, val float32) {
	if !event.AddFloat32(key, val) {
		event.AddField(key, val)
	}
}

// formatTimestamp uses the same behavior as protobuf's timestamp.
// "1972-01-01T10:00:20.021Z"	Uses RFC 3339, where generated output will always be Z-normalized and uses 0, 3, 6 or 9 fractional digits. Offsets other than "Z" are also accepted.
func formatTimestamp(t time.Time) string {
	t = t.UTC()
	x := t.Format("2006-01-02T15:04:05.000000000") // RFC 3339
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, ".000")
	return x + "Z"
}

// formatDuration uses the same behavior as protobuf's duration.
// "1.000340012s", "1s"	Generated output always contains 0, 3, 6, or 9 fractional digits, depending on required precision, followed by the suffix "s". Accepted are any fractional digits (also none) as long as they fit into nano-seconds precision and the suffix "s" is required.
func formatDuration(d time.Duration) string {
	nanos := d.Nanoseconds()
	secs := nanos / 1e9
	nanos -= secs * 1e9
	//if nanos <= -1e9 || nanos >= 1e9 || (secs > 0 && nanos < 0) || (secs < 0 && nanos > 0) {
	//	panic("invalid duration")
	//}
	sign := ""
	if secs < 0 || nanos < 0 {
		sign, secs, nanos = "-", -1*secs, -1*nanos
	}
	x := fmt.Sprintf("%s%d.%09d", sign, secs, nanos)
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, "000")
	x = strings.TrimSuffix(x, ".000")
	return x + "s"
}

// Implementations of field modifiers / builders.
// All together, to make it easier to ensure both Context and Builder implement the same set of methods.

func (x modifierMethods[E]) Field(event E, key string, val any) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	switch val := val.(type) {
	case string:
		x.str(event, key, val)
	case []byte:
		x.bytes(event, key, val)
	case time.Time:
		x.time(event, key, val)
	case time.Duration:
		x.duration(event, key, val)
	case int:
		x.int(event, key, val)
	case float32:
		x.float32(event, key, val)
	default:
		event.AddField(key, val)
	}
	return nil
}

// Field adds a field to the log context, making an effort to choose the most
// appropriate handler for the value.
//
// WARNING: The behavior of this method may change without notice.
//
// Use the Interface method if you want a direct pass-through to the
// Event.AddField implementation.
func (x *Context[E]) Field(key string, val any) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Field(event, key, val) })
	}
	return x
}

// Field adds a field to the log event, making an effort to choose the most
// appropriate handler for the value.
//
// WARNING: The behavior of this method may change without notice.
//
// Use the Interface method if you want a direct pass-through to the
// Event.AddField implementation.
func (x *Builder[E]) Field(key string, val any) *Builder[E] {
	if x.ok() {
		_ = x.methods.Field(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Interface(event E, key string, val any) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	event.AddField(key, val)
	return nil
}

// Interface adds a structured log field, which will pass through to
// Event.AddField.
func (x *Context[E]) Interface(key string, val any) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Interface(event, key, val) })
	}
	return x
}

// Interface adds a structured log field, which will pass through to
// Event.AddField.
func (x *Builder[E]) Interface(key string, val any) *Builder[E] {
	if x.ok() {
		_ = x.methods.Interface(x.Event, key, val)
	}
	return x
}

// Any is an alias for [Context.Interface].
func (x *Context[E]) Any(key string, val any) *Context[E] { return x.Interface(key, val) }

// Any is an alias for [Builder.Interface].
func (x *Builder[E]) Any(key string, val any) *Builder[E] { return x.Interface(key, val) }

func (x modifierMethods[E]) Err(event E, err error) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	if !event.AddError(err) {
		event.AddField(`err`, err)
	}
	return nil
}

// Err adds an error as a structured log field, the key for which will either
// be determined by the Event.AddError method, or will be "err" if not
// implemented.
func (x *Context[E]) Err(err error) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Err(event, err) })
	}
	return x
}

// Err adds an error as a structured log field, the key for which will either
// be determined by the Event.AddError method, or will be "err" if not
// implemented.
func (x *Builder[E]) Err(err error) *Builder[E] {
	if x.ok() {
		_ = x.methods.Err(x.Event, err)
	}
	return x
}

func (x modifierMethods[E]) Str(event E, key string, val string) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.str(event, key, val)
	return nil
}

// Str adds a string as a structured log field, using Event.AddString if
// available, otherwise falling back to Event.AddField.
func (x *Context[E]) Str(key string, val string) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Str(event, key, val) })
	}
	return x
}

// Str adds a string as a structured log field, using Event.AddString if
// available, otherwise falling back to Event.AddField.
func (x *Builder[E]) Str(key string, val string) *Builder[E] {
	if x.ok() {
		_ = x.methods.Str(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Int(event E, key string, val int) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.int(event, key, val)
	return nil
}

// Int adds an int as a structured log field, using Event.AddInt if available,
// otherwise falling back to Event.AddField.
func (x *Context[E]) Int(key string, val int) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Int(event, key, val) })
	}
	return x
}

// Int adds an int as a structured log field, using Event.AddInt if available,
// otherwise falling back to Event.AddField.
func (x *Builder[E]) Int(key string, val int) *Builder[E] {
	if x.ok() {
		_ = x.methods.Int(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Float32(event E, key string, val float32) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.float32(event, key, val)
	return nil
}

// Float32 adds a float32 as a structured log field, using Event.AddFloat32 if
// available, otherwise falling back to Event.AddField.
func (x *Context[E]) Float32(key string, val float32) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Float32(event, key, val) })
	}
	return x
}

// Float32 adds a float32 as a structured log field, using Event.AddFloat32 if
// available, otherwise falling back to Event.AddField.
func (x *Builder[E]) Float32(key string, val float32) *Builder[E] {
	if x.ok() {
		_ = x.methods.Float32(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Time(event E, key string, t time.Time) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.time(event, key, t)
	return nil
}

// Time adds a time.Time as a structured log field, using Event.AddTime if
// available, otherwise falling back to formatting the time.Time as a string in
// the RFC 3339 format, using the same semantics as the JSON encoding of
// Protobuf's "well known type", google.protobuf.Timestamp. In this fallback
// case, the behavior of [Context.Str] is used, to add the field.
//
// See also
// [https://github.com/protocolbuffers/protobuf/blob/4f6ef7e4d88a74dfcd82b36ef46844b22b9e54b1/src/google/protobuf/timestamp.proto].
func (x *Context[E]) Time(key string, t time.Time) *Context[E] {
	if x.ok() {
		x.add(func(event E) error { return x.methods.Time(event, key, t) })
	}
	return x
}

// Time adds a time.Time as a structured log field, using Event.AddTime if
// available, otherwise falling back to formatting the time.Time as a string in
// the RFC 3339 format, using the same semantics as the JSON encoding of
// Protobuf's "well known type", google.protobuf.Timestamp. In this fallback
// case, the behavior of [Builder.Str] is used, to add the field.
//
// See also
// [https://github.com/protocolbuffers/protobuf/blob/4f6ef7e4d88a74dfcd82b36ef46844b22b9e54b1/src/google/protobuf/timestamp.proto].
func (x *Builder[E]) Time(key string, t time.Time) *Builder[E] {
	if x.ok() {
		_ = x.methods.Time(x.Event, key, t)
	}
	return x
}
