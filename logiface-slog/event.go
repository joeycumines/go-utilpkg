package slog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"runtime"
	"time"

	"log/slog"

	"github.com/joeycumines/logiface"
)

//lint:ignore U1000 embedded Event type for its methods from UnimplementedEvent
//lint:file-ignore U1000 field order optimized for memory patterns
type Event struct {
	//lint:ignore U1000 embedded for its methods
	logiface.UnimplementedEvent

	// time is the timestamp for this log event
	time time.Time

	// ctx is the context associated with this log event
	ctx context.Context

	// err is any error set via AddError
	err error

	// logger is the parent Logger that created this event
	logger *Logger

	// message is the log message string
	message string

	// attrs accumulates slog.Attr values set via Add* methods
	attrs []slog.Attr

	// groups tracks the current group stack for nested attributes
	groups []string

	// slogLevel is the slog.Level for this event
	slogLevel slog.Level
}

// Level returns the logiface.Level for this event.
// Note: This is a lossy conversion from slogLevel, as slog has only 4 levels.
func (x *Event) Level() logiface.Level {
	if x == nil {
		return logiface.LevelDisabled
	}
	return toLogifaceLevel(x.slogLevel)
}

// Reset clears all event fields for reuse in the pool.
// IMPORTANT: This must be called before returning an event to sync.Pool.
func (x *Event) Reset() {
	if x == nil {
		return
	}
	// Clear references to prevent memory leaks
	x.logger = nil
	x.ctx = nil

	// Reset fields
	x.slogLevel = 0
	x.time = time.Time{}
	x.message = ""
	x.err = nil

	// Reset slices to zero length, preserve capacity for reuse
	x.attrs = x.attrs[:0]
	x.groups = x.groups[:0]
}

// Send emits the event via the underlying slog.Handler.
// It constructs a slog.Record from the accumulated data and calls Handle.
// After emitting, the event should be released back to the pool.
func (x *Event) Send() error {
	if x == nil || x.logger == nil {
		return nil
	}

	// Check if level is enabled before constructing Record
	if !x.logger.handler.Enabled(x.ctx, x.slogLevel) {
		return logiface.ErrDisabled
	}

	// Capture PC for source location (skip 2: Send(), then runtime.Callers)
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	pc := pcs[0]

	// Construct slog.Record
	record := slog.NewRecord(
		x.time,
		x.slogLevel,
		x.message,
		pc,
	)

	// Add all accumulated attributes
	for _, attr := range x.attrs {
		record.Add(attr)
	}

	// Add error if present
	if x.err != nil {
		record.Add(slog.Any("error", x.err))
	}

	// Handle via slog.Handler
	return x.logger.handler.Handle(x.ctx, record)
}

// AddMessage sets the log message for this event.
func (x *Event) AddMessage(msg string) bool {
	if x == nil {
		return false
	}
	x.message = msg
	return true
}

// AddField adds a generic field to the event using slog.Any.
func (x *Event) AddField(key string, val any) {
	if x == nil {
		return
	}
	x.attrs = append(x.attrs, slog.Any(key, val))
}

// AddError adds an error field to the event.
func (x *Event) AddError(err error) bool {
	if x == nil {
		return false
	}
	x.err = err
	return true
}

// AddString adds a string field to the event.
func (x *Event) AddString(key string, val string) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.String(key, val))
	return true
}

// AddInt adds an int field to the event.
func (x *Event) AddInt(key string, val int) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Int64(key, int64(val)))
	return true
}

// AddInt8 adds an int8 field to the event.
func (x *Event) AddInt8(key string, val int8) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Int64(key, int64(val)))
	return true
}

// AddInt16 adds an int16 field to the event.
func (x *Event) AddInt16(key string, val int16) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Int64(key, int64(val)))
	return true
}

// AddInt32 adds an int32 field to the event.
func (x *Event) AddInt32(key string, val int32) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Int64(key, int64(val)))
	return true
}

// AddInt64 adds an int64 field to the event.
func (x *Event) AddInt64(key string, val int64) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Int64(key, val))
	return true
}

// AddUint adds a uint field to the event.
func (x *Event) AddUint(key string, val uint) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Uint64(key, uint64(val)))
	return true
}

// AddUint8 adds a uint8 field to the event.
func (x *Event) AddUint8(key string, val uint8) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Uint64(key, uint64(val)))
	return true
}

// AddUint16 adds a uint16 field to the event.
func (x *Event) AddUint16(key string, val uint16) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Uint64(key, uint64(val)))
	return true
}

// AddUint32 adds a uint32 field to the event.
func (x *Event) AddUint32(key string, val uint32) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Uint64(key, uint64(val)))
	return true
}

// AddUint64 adds a uint64 field to the event.
func (x *Event) AddUint64(key string, val uint64) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Uint64(key, val))
	return true
}

// AddFloat32 adds a float32 field to the event.
func (x *Event) AddFloat32(key string, val float32) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Float64(key, float64(val)))
	return true
}

// AddFloat64 adds a float64 field to the event.
func (x *Event) AddFloat64(key string, val float64) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Float64(key, val))
	return true
}

// AddBool adds a boolean field to the event.
func (x *Event) AddBool(key string, val bool) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Bool(key, val))
	return true
}

// AddTime adds a time.Time field to the event.
func (x *Event) AddTime(key string, val time.Time) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Time(key, val))
	return true
}

// AddDuration adds a time.Duration field to the event.
func (x *Event) AddDuration(key string, val time.Duration) bool {
	if x == nil {
		return false
	}
	x.attrs = append(x.attrs, slog.Duration(key, val))
	return true
}

// AddBase64Bytes adds a base64-encoded byte slice field to the event.
func (x *Event) AddBase64Bytes(key string, val []byte, enc *base64.Encoding) bool {
	if x == nil {
		return false
	}
	if enc == nil {
		enc = base64.StdEncoding
	}
	x.attrs = append(x.attrs, slog.String(key, enc.EncodeToString(val)))
	return true
}

// AddRawJSON adds a raw JSON field to the event.
// It attempts to parse the JSON and add it with appropriate type.
func (x *Event) AddRawJSON(key string, val json.RawMessage) bool {
	if x == nil {
		return false
	}
	var v any
	if err := json.Unmarshal(val, &v); err == nil {
		// Successfully parsed, add as appropriate type
		x.attrs = append(x.attrs, slog.Any(key, v))
	} else {
		// Parsing failed, fall back to string representation
		x.attrs = append(x.attrs, slog.String(key, string(val)))
	}
	return true
}
