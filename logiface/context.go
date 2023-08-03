package logiface

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	_ builderMode = 1 << iota >> 1
	builderModePanic
	builderModeFatal
	// apply category rate limit, based on the caller (runtime)
	// see also limit.go
	builderModeCallerCategoryRateLimit
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
		methods   modifierMethods[E]
		logger    *Logger[E]
		Modifiers ModifierSlice[E]
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

		// WARNING: If additional fields are added, they may need to be reset
		// (see Builder.Release)

		methods modifierMethods[E]
		shared  *loggerShared[E]

		// mode provides switching behavior in the form of bit flags
		mode builderMode
	}

	modifierMethods[E Event] struct{}

	// builderMode models bit flags for [Builder], for special behavior
	builderMode int32
)

// Logger returns the underlying (sub)logger, or nil.
//
// Note that the returned logger will apply all parent modifiers, including
// [Context.Modifiers]. This method is intended to be used to get the actual
// sub-logger, after building the context that sub-logger is to apply.
//
// This method is not implemented by [Builder].
func (x *Context[E]) Logger() *Logger[E] {
	if x == nil {
		return nil
	}
	return x.logger
}

// Call is provided as a convenience, to facilitate code which uses the
// receiver explicitly, without breaking out of the fluent-style API.
// The provided fn will not be called if not [Context.Enabled].
func (x *Context[E]) Call(fn func(b *Context[E])) *Context[E] {
	if x.Enabled() {
		fn(x)
	}
	return x
}

// Modifier appends val to the receiver, if the receiver is enabled, and val is
// non-nil.
func (x *Context[E]) Modifier(val Modifier[E]) *Context[E] {
	if x.Enabled() && val != nil {
		x.Modifiers = append(x.Modifiers, val)
	}
	return x
}

func (x *Context[E]) add(fn ModifierFunc[E]) {
	x.Modifiers = append(x.Modifiers, fn)
}

// Root returns the root [Logger] for this instance.
func (x *Context[E]) Root() *Logger[E] {
	if x != nil {
		return x.logger.Root()
	}
	return nil
}

// Modifier calls val.Modify, if the receiver is enabled, and val is non-nil.
// If the modifier returns [ErrDisabled] or [ErrLimited], the return value will
// be nil, and the receiver will be released.
// If the modifier returns any other non-nil error, [Logger.DPanic] will be
// called.
func (x *Builder[E]) Modifier(val Modifier[E]) *Builder[E] {
	if x.Enabled() && val != nil {
		if err := val.Modify(x.Event); err != nil {
			switch {
			case errors.Is(err, ErrDisabled):
			case errors.Is(err, ErrLimited):
			default:
				x.shared.root.DPanic().
					Err(err).
					Log("modifier error")
			}
			x.releaseAll()
			return nil
		}
	}
	return x
}

// Call is provided as a convenience, to facilitate code which uses the
// receiver explicitly, without breaking out of the fluent-style API.
// The provided fn will not be called if not [Builder.Enabled].
//
// Example use cases include access of the underlying Event implementation (to
// utilize functionality not mapped by logiface), or to skip building /
// formatting certain fields, based on if `b.Event.Level().Enabled()`.
func (x *Builder[E]) Call(fn func(b *Builder[E])) *Builder[E] {
	if x.Enabled() {
		fn(x)
	}
	return x
}

// Log logs an event, with the given message, using the provided level and
// modifier, if the relevant conditions are met (e.g. configured log level).
//
// The field for the message will either be determined by the implementation,
// if Event.AddMessage is implemented, or the default field name "msg" will be
// used.
//
// This method calls [Builder.Release].
// This method is not implemented by [Context].
func (x *Builder[E]) Log(msg string) {
	if !x.Enabled() {
		return
	}
	defer x.releaseAll()
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
// This method calls [Builder.Release].
// This method is not implemented by [Context].
func (x *Builder[E]) Logf(format string, args ...any) {
	if !x.Enabled() {
		return
	}
	defer x.releaseAll()
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
// This method calls [Builder.Release].
// This method is not implemented by [Context].
func (x *Builder[E]) LogFunc(fn func() string) {
	if !x.Enabled() {
		return
	}
	defer x.releaseAll()
	if x.Event.Level().Enabled() {
		x.log(fn())
	}
}

func (x *Builder[E]) log(msg string) {
	if (x.mode & builderModeCallerCategoryRateLimit) == builderModeCallerCategoryRateLimit {
		// skip 2 because there's this method + the (exported) caller of this method
		caller, next, ok := x.shared.catrateAllowCaller(2)
		if !ok {
			return
		}
		if next != (time.Time{}) {
			x.attachCallerRateLimitWarning(caller, next)
		}
	}
	if msg != `` && !x.Event.AddMessage(msg) {
		x.Event.AddField(`msg`, msg)
	}
	_ = x.shared.writer.Write(x.Event)
	if x.mode != 0 {
		if (x.mode & builderModePanic) == builderModePanic {
			if msg == `` {
				panic(`logiface: panic requested`)
			}
			panic(msg)
		}
		if (x.mode & builderModeFatal) == builderModeFatal {
			OsExit(1)
		}
	}
}

// Release returns the Builder to the pool, calling any user-defined
// EventReleaser to reset or release [Builder.Event] (e.g. if the concrete
// event implementation also uses a pool).
//
// In most cases, it should not be necessary to call this directly. This method
// is exported to allow for more advanced use cases, such as when the Builder
// is used to build an event that is not logged, or when the Builder is used
// to build an event that is logged by a different logger implementation.
//
// This method is called by other "terminal" methods, such as [Builder.Log].
// This method is not implemented by [Context].
func (x *Builder[E]) Release() {
	if x.Enabled() {
		x.releaseAll()
	}
}

func (x *Builder[E]) releaseAll() {
	x.release(true)
}

func (x *Builder[E]) release(event bool) {
	if shared := x.shared; shared != nil {
		x.shared = nil
		if event && shared.releaser != nil {
			shared.releaser.ReleaseEvent(x.Event)
		}
		// clear the event value, in case it retains a reference
		// this is important in cases where the E value is also pooled
		x.Event = *new(E)
		// reset the remaining state...
		x.mode = 0
		// ...and return to the pool
		shared.pool.Put(x)
	}
}

// Root returns the root [Logger] for this instance.
func (x *Builder[E]) Root() *Logger[E] {
	if x != nil && x.shared != nil {
		return x.shared.root
	}
	return nil
}

func (x modifierMethods[E]) str(event E, key string, val string) {
	if !event.AddString(key, val) {
		event.AddField(key, val)
	}
}

func (x modifierMethods[E]) base64(event E, key string, val []byte, enc *base64.Encoding) {
	if enc == nil {
		enc = base64.StdEncoding
	}
	if !event.AddBase64Bytes(key, val, enc) {
		x.str(event, key, enc.EncodeToString(val))
	}
}

func (x modifierMethods[E]) time(event E, key string, val time.Time) {
	if !event.AddTime(key, val) {
		x.str(event, key, formatTimestamp(val))
	}
}

func (x modifierMethods[E]) dur(event E, key string, val time.Duration) {
	if !event.AddDuration(key, val) {
		x.str(event, key, formatDuration(val))
	}
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

func (x modifierMethods[E]) bool(event E, key string, val bool) {
	if !event.AddBool(key, val) {
		event.AddField(key, val)
	}
}

func (x modifierMethods[E]) float64(event E, key string, val float64) {
	if !event.AddFloat64(key, val) {
		event.AddField(key, val)
	}
}

func (x modifierMethods[E]) int64(event E, key string, val int64) {
	if !event.AddInt64(key, val) {
		x.str(event, key, strconv.FormatInt(val, 10))
	}
}

func (x modifierMethods[E]) uint64(event E, key string, val uint64) {
	if !event.AddUint64(key, val) {
		x.str(event, key, strconv.FormatUint(val, 10))
	}
}

func (x modifierMethods[E]) rawJSON(event E, key string, val json.RawMessage) {
	if !event.AddRawJSON(key, val) {
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

// Implementations of non-field methods that are shared between Context and Builder.

// Enabled indicates that this [Context] was initialized from a writable
// [Logger], via [Logger.Clone].
func (x *Context[E]) Enabled() bool {
	return x != nil && x.logger != nil
}

// Enabled indicates that this [Builder] was initialized by a [Logger], using a
// writable [Level], given the logger's configuration.
func (x *Builder[E]) Enabled() bool {
	return x != nil && x.shared != nil
}

// Implementations of field modifiers / builders.
// All together, to make it easier to ensure both Context and Builder implement the same set of methods.

func (x modifierMethods[E]) Field(event E, key string, val any) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	// TODO add cases for pointers
	switch val := val.(type) {
	case string:
		x.str(event, key, val)
	case []byte:
		x.base64(event, key, val, nil)
	case time.Time:
		x.time(event, key, val)
	case time.Duration:
		x.dur(event, key, val)
	case int:
		x.int(event, key, val)
	case float32:
		x.float32(event, key, val)
	case bool:
		x.bool(event, key, val)
	case float64:
		x.float64(event, key, val)
	case int64:
		x.int64(event, key, val)
	case uint64:
		x.uint64(event, key, val)
	case json.RawMessage:
		x.rawJSON(event, key, val)
	default:
		event.AddField(key, val)
	}
	return nil
}

// Field adds a field to the log context, making an effort to choose the most
// appropriate handler for the value.
//
// WARNING: The behavior of this method may change without notice, to
// facilitate the addition of new field types.
//
// Use the Interface method if you want a direct pass-through to the
// Event.AddField implementation.
func (x *Context[E]) Field(key string, val any) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Field(event, key, val) })
	}
	return x
}

// Field adds a field to the log event, making an effort to choose the most
// appropriate handler for the value.
//
// WARNING: The behavior of this method may change without notice, to
// facilitate the addition of new field types.
//
// Use the Interface method if you want a direct pass-through to the
// Event.AddField implementation.
func (x *Builder[E]) Field(key string, val any) *Builder[E] {
	if x.Enabled() {
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
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Interface(event, key, val) })
	}
	return x
}

// Interface adds a structured log field, which will pass through to
// Event.AddField.
func (x *Builder[E]) Interface(key string, val any) *Builder[E] {
	if x.Enabled() {
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
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Err(event, err) })
	}
	return x
}

// Err adds an error as a structured log field, the key for which will either
// be determined by the Event.AddError method, or will be "err" if not
// implemented.
func (x *Builder[E]) Err(err error) *Builder[E] {
	if x.Enabled() {
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
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Str(event, key, val) })
	}
	return x
}

// Str adds a string as a structured log field, using Event.AddString if
// available, otherwise falling back to Event.AddField.
func (x *Builder[E]) Str(key string, val string) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Str(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Stringer(event E, key string, val fmt.Stringer) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	if val == nil {
		x.str(event, key, `<nil>`)
	} else {
		x.str(event, key, val.String())
	}
	return nil
}

// Stringer adds a string as a structured log field, using Event.AddString if
// available, otherwise falling back to Event.AddField. Nil values will be
// encoded as `<nil>`.
func (x *Context[E]) Stringer(key string, val fmt.Stringer) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Stringer(event, key, val) })
	}
	return x
}

// Stringer adds a string as a structured log field, using Event.AddString if
// available, otherwise falling back to Event.AddField. Nil values will be
// encoded as `<nil>`.
func (x *Builder[E]) Stringer(key string, val fmt.Stringer) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Stringer(x.Event, key, val)
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
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Int(event, key, val) })
	}
	return x
}

// Int adds an int as a structured log field, using Event.AddInt if available,
// otherwise falling back to Event.AddField.
func (x *Builder[E]) Int(key string, val int) *Builder[E] {
	if x.Enabled() {
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
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Float32(event, key, val) })
	}
	return x
}

// Float32 adds a float32 as a structured log field, using Event.AddFloat32 if
// available, otherwise falling back to Event.AddField.
func (x *Builder[E]) Float32(key string, val float32) *Builder[E] {
	if x.Enabled() {
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
	if x.Enabled() {
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
	if x.Enabled() {
		_ = x.methods.Time(x.Event, key, t)
	}
	return x
}

func (x modifierMethods[E]) Dur(event E, key string, d time.Duration) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.dur(event, key, d)
	return nil
}

// Dur adds a time.Duration as a structured log field, using
// Event.AddDuration if available, otherwise falling back to formatting the
// time.Duration as a string, formatted as a decimal in seconds (with unit),
// using the same semantics as the JSON encoding of Protobuf's "well known
// type", google.protobuf.Duration. In this fallback case, the behavior of
// [Context.Str] is used, to add the field.
//
// See also
// [https://github.com/protocolbuffers/protobuf/blob/4f6ef7e4d88a74dfcd82b36ef46844b22b9e54b1/src/google/protobuf/duration.proto].
func (x *Context[E]) Dur(key string, d time.Duration) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Dur(event, key, d) })
	}
	return x
}

// Dur adds a time.Duration as a structured log field, using
// Event.AddDuration if available, otherwise falling back to formatting the
// time.Duration as a string, formatted as a decimal in seconds (with unit),
// using the same semantics as the JSON encoding of Protobuf's "well known
// type", google.protobuf.Duration. In this fallback case, the behavior of
// [Builder.Str] is used, to add the field.
//
// See also
// [https://github.com/protocolbuffers/protobuf/blob/4f6ef7e4d88a74dfcd82b36ef46844b22b9e54b1/src/google/protobuf/duration.proto].
func (x *Builder[E]) Dur(key string, d time.Duration) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Dur(x.Event, key, d)
	}
	return x
}

func (x modifierMethods[E]) Base64(event E, key string, b []byte, enc *base64.Encoding) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.base64(event, key, b, enc)
	return nil
}

// Base64 adds a []byte as a structured log field, using [Event.AddBase64Bytes]
// if available, otherwise falling back to directly encoding the []byte as a
// base64 string. The fallback behavior is intended to be the core behavior,
// with the [Event.AddBase64Bytes] method being an optimization.
//
// If enc is nil or base64.StdEncoding, the behavior is the same as the JSON
// encoding of Protobuf's bytes scalar.
// See also [https://protobuf.dev/programming-guides/proto3/#json].
func (x *Context[E]) Base64(key string, b []byte, enc *base64.Encoding) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Base64(event, key, b, enc) })
	}
	return x
}

// Base64 adds a []byte as a structured log field, using [Event.AddBase64Bytes]
// if available, otherwise falling back to directly encoding the []byte as a
// base64 string. The fallback behavior is intended to be the core behavior,
// with the [Event.AddBase64Bytes] method being an optimization.
//
// If enc is nil or base64.StdEncoding, the behavior is the same as the JSON
// encoding of Protobuf's bytes scalar.
// See also [https://protobuf.dev/programming-guides/proto3/#json].
func (x *Builder[E]) Base64(key string, b []byte, enc *base64.Encoding) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Base64(x.Event, key, b, enc)
	}
	return x
}

func (x modifierMethods[E]) Bool(event E, key string, val bool) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.bool(event, key, val)
	return nil
}

// Bool adds a bool as a structured log field, using Event.AddBool if
// available, otherwise falling back to Event.AddField.
func (x *Context[E]) Bool(key string, val bool) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Bool(event, key, val) })
	}
	return x
}

// Bool adds a bool as a structured log field, using Event.AddBool if
// available, otherwise falling back to Event.AddField.
func (x *Builder[E]) Bool(key string, val bool) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Bool(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Float64(event E, key string, val float64) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.float64(event, key, val)
	return nil
}

// Float64 adds a float64 as a structured log field, using Event.AddFloat64 if
// available, otherwise falling back to Event.AddField.
func (x *Context[E]) Float64(key string, val float64) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Float64(event, key, val) })
	}
	return x
}

// Float64 adds a float64 as a structured log field, using Event.AddFloat64 if
// available, otherwise falling back to Event.AddField.
func (x *Builder[E]) Float64(key string, val float64) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Float64(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Int64(event E, key string, val int64) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.int64(event, key, val)
	return nil
}

// Int64 adds an int64 as a structured log field, using Event.AddInt64 if
// available, otherwise falling back to encoding val as a decimal string, in
// the same manner as Protobuf's JSON encoding.
//
// For the fallback, the behavior of [Context.Str] is used, to add the field.
func (x *Context[E]) Int64(key string, val int64) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Int64(event, key, val) })
	}
	return x
}

// Int64 adds an int64 as a structured log field, using Event.AddInt64 if
// available, otherwise falling back to encoding val as a decimal string, in
// the same manner as Protobuf's JSON encoding.
//
// For the fallback, the behavior of [Builder.Str] is used, to add the field.
func (x *Builder[E]) Int64(key string, val int64) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Int64(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) Uint64(event E, key string, val uint64) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.uint64(event, key, val)
	return nil
}

// Uint64 adds an uint64 as a structured log field, using Event.AddUint64 if
// available, otherwise falling back to encoding val as a decimal string, in
// the same manner as Protobuf's JSON encoding.
//
// For the fallback, the behavior of [Context.Str] is used, to add the field.
func (x *Context[E]) Uint64(key string, val uint64) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.Uint64(event, key, val) })
	}
	return x
}

// Uint64 adds an uint64 as a structured log field, using Event.AddUint64 if
// available, otherwise falling back to encoding val as a decimal string, in
// the same manner as Protobuf's JSON encoding.
//
// For the fallback, the behavior of [Builder.Str] is used, to add the field.
func (x *Builder[E]) Uint64(key string, val uint64) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.Uint64(x.Event, key, val)
	}
	return x
}

func (x modifierMethods[E]) RawJSON(event E, key string, val json.RawMessage) error {
	if !event.Level().Enabled() {
		return ErrDisabled
	}
	x.rawJSON(event, key, val)
	return nil
}

// RawJSON adds a json.RawMessage as a structured log field, using
// Event.AddRawJSON if available, otherwise falling back to Event.AddField.
func (x *Context[E]) RawJSON(key string, val json.RawMessage) *Context[E] {
	if x.Enabled() {
		x.add(func(event E) error { return x.methods.RawJSON(event, key, val) })
	}
	return x
}

// RawJSON adds a json.RawMessage as a structured log field, using
// Event.AddRawJSON if available, otherwise falling back to Event.AddField.
func (x *Builder[E]) RawJSON(key string, val json.RawMessage) *Builder[E] {
	if x.Enabled() {
		_ = x.methods.RawJSON(x.Event, key, val)
	}
	return x
}
