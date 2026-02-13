package gojaprotobuf

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Well-known type descriptors (resolved once).
var (
	timestampDesc = timestamppb.File_google_protobuf_timestamp_proto.Messages().ByName("Timestamp")
	durationDesc  = durationpb.File_google_protobuf_duration_proto.Messages().ByName("Duration")
	anyDesc       = anypb.File_google_protobuf_any_proto.Messages().ByName("Any")
)

// jsTimestampNow is the JS-facing implementation of pb.timestampNow().
// It creates a google.protobuf.Timestamp for the current time.
func (m *Module) jsTimestampNow(call goja.FunctionCall) goja.Value {
	now := time.Now()
	return m.wrapMessage(timestampFromTime(now))
}

// jsTimestampFromDate is the JS-facing implementation of
// pb.timestampFromDate(date). It creates a google.protobuf.Timestamp
// from a JavaScript Date object (milliseconds since epoch).
func (m *Module) jsTimestampFromDate(call goja.FunctionCall) goja.Value {
	val := call.Argument(0)
	ms, err := m.extractDateMs(val)
	if err != nil {
		panic(m.runtime.NewTypeError("timestampFromDate: %s", err))
	}
	return m.wrapMessage(timestampFromMs(ms))
}

// jsTimestampDate is the JS-facing implementation of
// pb.timestampDate(ts). It converts a google.protobuf.Timestamp to a
// JavaScript Date object.
func (m *Module) jsTimestampDate(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("timestampDate: %s", err))
	}
	ms := timestampToMs(msg)
	return m.newDate(ms)
}

// jsTimestampFromMs is the JS-facing implementation of
// pb.timestampFromMs(ms). It creates a google.protobuf.Timestamp from
// milliseconds since the Unix epoch.
func (m *Module) jsTimestampFromMs(call goja.FunctionCall) goja.Value {
	ms := call.Argument(0).ToInteger()
	return m.wrapMessage(timestampFromMs(ms))
}

// jsTimestampMs is the JS-facing implementation of pb.timestampMs(ts).
// It returns the Unix epoch milliseconds from a Timestamp message.
func (m *Module) jsTimestampMs(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("timestampMs: %s", err))
	}
	return m.runtime.ToValue(timestampToMs(msg))
}

// jsDurationFromMs is the JS-facing implementation of
// pb.durationFromMs(ms). It creates a google.protobuf.Duration from
// a value in milliseconds.
func (m *Module) jsDurationFromMs(call goja.FunctionCall) goja.Value {
	ms := call.Argument(0).ToInteger()
	return m.wrapMessage(durationFromMs(ms))
}

// jsDurationMs is the JS-facing implementation of pb.durationMs(dur).
// It returns the milliseconds from a Duration message.
func (m *Module) jsDurationMs(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("durationMs: %s", err))
	}
	return m.runtime.ToValue(durationToMs(msg))
}

// jsAnyPack is the JS-facing implementation of pb.anyPack(msgType, msg).
// It wraps a protobuf message into a google.protobuf.Any.
func (m *Module) jsAnyPack(call goja.FunctionCall) goja.Value {
	msgTypeVal := call.Argument(0)
	msgDesc, err := m.extractMessageDesc(msgTypeVal)
	if err != nil {
		panic(m.runtime.NewTypeError("anyPack: first argument: %s", err))
	}

	msg, err := m.unwrapMessage(call.Argument(1))
	if err != nil {
		panic(m.runtime.NewTypeError("anyPack: second argument: %s", err))
	}

	// Verify message type matches.
	if msg.Descriptor().FullName() != msgDesc.FullName() {
		panic(m.runtime.NewTypeError("anyPack: message type %q does not match schema %q",
			msg.Descriptor().FullName(), msgDesc.FullName()))
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		panic(m.runtime.NewGoError(fmt.Errorf("anyPack: marshal: %w", err)))
	}

	anyMsg := dynamicpb.NewMessage(anyDesc)
	typeURL := "type.googleapis.com/" + string(msgDesc.FullName())
	anyMsg.Set(anyDesc.Fields().ByName("type_url"), protoreflect.ValueOfString(typeURL))
	anyMsg.Set(anyDesc.Fields().ByName("value"), protoreflect.ValueOfBytes(data))

	return m.wrapMessage(anyMsg)
}

// jsAnyUnpack is the JS-facing implementation of
// pb.anyUnpack(anyMsg, msgType). It extracts a message from a
// google.protobuf.Any.
func (m *Module) jsAnyUnpack(call goja.FunctionCall) goja.Value {
	anyMsg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("anyUnpack: first argument: %s", err))
	}

	msgDesc, err := m.extractMessageDesc(call.Argument(1))
	if err != nil {
		panic(m.runtime.NewTypeError("anyUnpack: second argument: %s", err))
	}

	// Extract value bytes.
	valueField := anyDesc.Fields().ByName("value")
	data := anyMsg.Get(valueField).Bytes()

	msg := dynamicpb.NewMessage(msgDesc)
	if err := proto.Unmarshal(data, msg); err != nil {
		panic(m.runtime.NewGoError(fmt.Errorf("anyUnpack: unmarshal: %w", err)))
	}

	return m.wrapMessage(msg)
}

// jsAnyIs is the JS-facing implementation of
// pb.anyIs(anyMsg, typeName). It checks whether an Any message
// contains a specific type. typeName can be a string or a message
// type constructor.
func (m *Module) jsAnyIs(call goja.FunctionCall) goja.Value {
	anyMsg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("anyIs: first argument: %s", err))
	}

	typeURLField := anyDesc.Fields().ByName("type_url")
	typeURL := anyMsg.Get(typeURLField).String()

	// Extract the type name from the URL (after last /).
	var gotName string
	if idx := lastSlash(typeURL); idx >= 0 {
		gotName = typeURL[idx+1:]
	} else {
		gotName = typeURL
	}

	// Second argument: string type name or message type constructor.
	typeArg := call.Argument(1)
	if typeArg == nil || goja.IsUndefined(typeArg) || goja.IsNull(typeArg) {
		panic(m.runtime.NewTypeError("anyIs: second argument must be a type name or message type"))
	}

	// Try as message type constructor first.
	if desc, descErr := m.extractMessageDesc(typeArg); descErr == nil {
		return m.runtime.ToValue(gotName == string(desc.FullName()))
	}

	// Fall back to string comparison.
	wantName := typeArg.String()
	return m.runtime.ToValue(gotName == wantName)
}

// ---------- Internal helpers ----------

// timestampFromTime creates a Timestamp dynamicpb.Message from a time.Time.
func timestampFromTime(t time.Time) *dynamicpb.Message {
	msg := dynamicpb.NewMessage(timestampDesc)
	msg.Set(timestampDesc.Fields().ByName("seconds"), protoreflect.ValueOfInt64(t.Unix()))
	msg.Set(timestampDesc.Fields().ByName("nanos"), protoreflect.ValueOfInt32(int32(t.Nanosecond())))
	return msg
}

// timestampFromMs creates a Timestamp dynamicpb.Message from epoch millis.
// Per the proto spec, nanos must be in [0, 999999999].
func timestampFromMs(ms int64) *dynamicpb.Message {
	msg := dynamicpb.NewMessage(timestampDesc)
	seconds := ms / 1000
	nanos := (ms % 1000) * 1_000_000
	// Normalize: Go's % truncates toward zero, but proto Timestamp
	// requires nanos in [0, 999999999]. For negative sub-second values,
	// adjust seconds down and nanos up.
	if nanos < 0 {
		seconds--
		nanos += 1_000_000_000
	}
	msg.Set(timestampDesc.Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	msg.Set(timestampDesc.Fields().ByName("nanos"), protoreflect.ValueOfInt32(int32(nanos)))
	return msg
}

// timestampToMs extracts epoch milliseconds from a Timestamp message.
func timestampToMs(msg *dynamicpb.Message) int64 {
	seconds := msg.Get(timestampDesc.Fields().ByName("seconds")).Int()
	nanos := msg.Get(timestampDesc.Fields().ByName("nanos")).Int()
	return seconds*1000 + nanos/1_000_000
}

// durationFromMs creates a Duration dynamicpb.Message from millis.
// Per the proto spec, seconds and nanos must have the same sign,
// with nanos in [-999999999, 999999999].
func durationFromMs(ms int64) *dynamicpb.Message {
	msg := dynamicpb.NewMessage(durationDesc)
	seconds := ms / 1000
	nanos := (ms % 1000) * 1_000_000
	// Go's % truncates toward zero, which already produces same-sign
	// nanos for durations. No normalization needed since the sign of
	// nanos matches the sign of seconds (both from the same ms value).
	msg.Set(durationDesc.Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	msg.Set(durationDesc.Fields().ByName("nanos"), protoreflect.ValueOfInt32(int32(nanos)))
	return msg
}

// durationToMs extracts milliseconds from a Duration message.
func durationToMs(msg *dynamicpb.Message) int64 {
	seconds := msg.Get(durationDesc.Fields().ByName("seconds")).Int()
	nanos := msg.Get(durationDesc.Fields().ByName("nanos")).Int()
	return seconds*1000 + nanos/1_000_000
}

// extractDateMs extracts milliseconds since epoch from a JS Date object
// or a numeric value.
func (m *Module) extractDateMs(val goja.Value) (int64, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return 0, fmt.Errorf("expected Date or number, got null/undefined")
	}

	// Try calling getTime() method (works for Date objects).
	obj := val.ToObject(m.runtime)
	getTimeVal := obj.Get("getTime")
	if getTimeVal != nil && !goja.IsUndefined(getTimeVal) {
		if fn, ok := goja.AssertFunction(getTimeVal); ok {
			result, err := fn(obj)
			if err != nil {
				return 0, fmt.Errorf("getTime() failed: %w", err)
			}
			return result.ToInteger(), nil
		}
	}

	// Fall back to numeric value.
	return val.ToInteger(), nil
}

// newDate creates a JavaScript Date from epoch milliseconds.
func (m *Module) newDate(ms int64) goja.Value {
	dateCtor := m.runtime.Get("Date")
	if dateCtor == nil || goja.IsUndefined(dateCtor) {
		// Fallback: return millis as number.
		return m.runtime.ToValue(ms)
	}
	result, err := m.runtime.New(dateCtor, m.runtime.ToValue(ms))
	if err != nil {
		// Fallback: return millis.
		return m.runtime.ToValue(ms)
	}
	return result
}

// lastSlash returns the index of the last '/' in s, or -1.
func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}
