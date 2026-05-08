package gojaprotobuf

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Well-known type descriptors (resolved once).
var (
	timestampType = (&timestamppb.Timestamp{}).ProtoReflect().Type()
	durationType  = (&durationpb.Duration{}).ProtoReflect().Type()
	anyType       = (&anypb.Any{}).ProtoReflect().Type()
	timestampDesc = timestampType.Descriptor()
	durationDesc  = durationType.Descriptor()
	anyDesc       = anyType.Descriptor()
)

func seedWellKnownState(state *runtimeState) error {
	var err error
	state.timestampType, err = seedWellKnownMessage(state, timestampType)
	if err != nil {
		return fmt.Errorf("configure %s: %w", timestampDesc.FullName(), err)
	}
	state.durationType, err = seedWellKnownMessage(state, durationType)
	if err != nil {
		return fmt.Errorf("configure %s: %w", durationDesc.FullName(), err)
	}
	state.anyType, err = seedWellKnownMessage(state, anyType)
	if err != nil {
		return fmt.Errorf("configure %s: %w", anyDesc.FullName(), err)
	}
	return nil
}

func seedWellKnownMessage(
	state *runtimeState,
	generated protoreflect.MessageType,
) (protoreflect.MessageType, error) {
	name := generated.Descriptor().FullName()
	baseType, typeFound, err := baseWellKnownType(state.baseTypes, name)
	if err != nil {
		return nil, err
	}
	baseDescriptor, fileFound, err := baseWellKnownDescriptor(state, name)
	if err != nil {
		return nil, err
	}
	if typeFound {
		if err := validateWellKnownDescriptor(baseType.Descriptor(), generated.Descriptor()); err != nil {
			return nil, fmt.Errorf("base type registry: %w", err)
		}
	}
	if fileFound {
		if err := validateWellKnownDescriptor(baseDescriptor, generated.Descriptor()); err != nil {
			return nil, fmt.Errorf("base file registry: %w", err)
		}
	}
	if typeFound && fileFound {
		if baseType.Descriptor() != baseDescriptor {
			return nil, fmt.Errorf("base registries disagree on symbol %q", name)
		}
		return baseType, nil
	}
	if typeFound {
		if err := registerWellKnownFile(state, baseType.Descriptor().ParentFile()); err != nil {
			return nil, err
		}
		return baseType, nil
	}
	if fileFound {
		messageType := dynamicpb.NewMessageType(baseDescriptor)
		if err := state.localTypes.RegisterMessage(messageType); err != nil {
			return nil, fmt.Errorf("register canonical message type: %w", err)
		}
		return messageType, nil
	}
	if err := registerWellKnownFile(state, generated.Descriptor().ParentFile()); err != nil {
		return nil, err
	}
	if err := state.localTypes.RegisterMessage(generated); err != nil {
		return nil, fmt.Errorf("register generated message type: %w", err)
	}
	return generated, nil
}

func baseWellKnownType(
	types *protoregistry.Types,
	name protoreflect.FullName,
) (protoreflect.MessageType, bool, error) {
	if values := registeredEnumValues(types, name); len(values) != 0 {
		return nil, false, fmt.Errorf("symbol %q conflicts with a base enum value", name)
	}
	messageType, err := types.FindMessageByName(name)
	if errors.Is(err, protoregistry.NotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("base type symbol %q is not a message: %w", name, err)
	}
	return messageType, true, nil
}

func baseWellKnownDescriptor(
	state *runtimeState,
	name protoreflect.FullName,
) (protoreflect.MessageDescriptor, bool, error) {
	descriptor, err := state.baseFiles.FindDescriptorByName(name)
	if errors.Is(err, protoregistry.NotFound) {
		var ok bool
		descriptor, ok = state.baseGraph.symbols[name]
		if !ok {
			return nil, false, nil
		}
		err = nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve base file symbol %q: %w", name, err)
	}
	messageDescriptor, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, false, fmt.Errorf("base file symbol %q is not a message", name)
	}
	return messageDescriptor, true, nil
}

func validateWellKnownDescriptor(
	descriptor protoreflect.MessageDescriptor,
	generated protoreflect.MessageDescriptor,
) error {
	if descriptor.FullName() != generated.FullName() ||
		!proto.Equal(
			protodesc.ToFileDescriptorProto(descriptor.ParentFile()),
			protodesc.ToFileDescriptorProto(generated.ParentFile()),
		) {
		return fmt.Errorf("symbol %q does not match the standard schema", generated.FullName())
	}
	return nil
}

func registerWellKnownFile(state *runtimeState, file protoreflect.FileDescriptor) error {
	if existing, ok := state.baseGraph.files[file.Path()]; ok && existing != file {
		return fmt.Errorf(
			"file %q conflicts with the base descriptor graph",
			file.Path(),
		)
	}
	fileGraph, err := descriptorFileGraph(file)
	if err != nil {
		return fmt.Errorf("validate canonical file %q: %w", file.Path(), err)
	}
	if err := state.baseGraph.compatible(fileGraph); err != nil {
		return err
	}
	if err := state.localFiles.RegisterFile(file); err != nil {
		return fmt.Errorf("register canonical file %q: %w", file.Path(), err)
	}
	state.localProtos[file.Path()] = protodesc.ToFileDescriptorProto(file)
	return nil
}

// jsTimestampNow is the JS-facing implementation of pb.timestampNow().
// It creates a google.protobuf.Timestamp for the current time.
func (m *Module) jsTimestampNow(call goja.FunctionCall) goja.Value {
	now := time.Now()
	message := timestampMessage(m.state.timestampType, now.Unix(), int32(now.Nanosecond()))
	if err := validateTimestampDescriptor(message, m.state.timestampType.Descriptor()); err != nil {
		panic(m.runtime.NewGoError(fmt.Errorf("timestampNow: %w", err)))
	}
	return m.wrapMessage(message)
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
	message := timestampMillis(m.state.timestampType, ms)
	if err := validateTimestampDescriptor(message, m.state.timestampType.Descriptor()); err != nil {
		panic(m.runtime.NewTypeError("timestampFromDate: %s", err))
	}
	return m.wrapMessage(message)
}

// jsTimestampDate is the JS-facing implementation of
// pb.timestampDate(ts). It converts a google.protobuf.Timestamp to a
// JavaScript Date object.
func (m *Module) jsTimestampDate(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("timestampDate: %s", err))
	}
	ms, err := checkedTimestampDescriptor(msg, m.state.timestampType.Descriptor())
	if err != nil {
		panic(m.runtime.NewTypeError("timestampDate: %s", err))
	}
	return m.newDate(ms)
}

// jsTimestampFromMs is the JS-facing implementation of
// pb.timestampFromMs(ms). It creates a google.protobuf.Timestamp from
// milliseconds since the Unix epoch.
func (m *Module) jsTimestampFromMs(call goja.FunctionCall) goja.Value {
	ms, err := m.gojaToInt64(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("timestampFromMs: %s", err))
	}
	message := timestampMillis(m.state.timestampType, ms)
	if err := validateTimestampDescriptor(message, m.state.timestampType.Descriptor()); err != nil {
		panic(m.runtime.NewTypeError("timestampFromMs: %s", err))
	}
	return m.wrapMessage(message)
}

// jsTimestampMs is the JS-facing implementation of pb.timestampMs(ts).
// It returns the Unix epoch milliseconds from a Timestamp message.
func (m *Module) jsTimestampMs(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("timestampMs: %s", err))
	}
	ms, err := checkedTimestampDescriptor(msg, m.state.timestampType.Descriptor())
	if err != nil {
		panic(m.runtime.NewTypeError("timestampMs: %s", err))
	}
	return m.int64ToGoja(ms)
}

// jsDurationFromMs is the JS-facing implementation of
// pb.durationFromMs(ms). It creates a google.protobuf.Duration from
// a value in milliseconds.
func (m *Module) jsDurationFromMs(call goja.FunctionCall) goja.Value {
	ms, err := m.gojaToInt64(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("durationFromMs: %s", err))
	}
	message := durationMillis(m.state.durationType, ms)
	if err := validateDurationDescriptor(message, m.state.durationType.Descriptor()); err != nil {
		panic(m.runtime.NewTypeError("durationFromMs: %s", err))
	}
	return m.wrapMessage(message)
}

// jsDurationMs is the JS-facing implementation of pb.durationMs(dur).
// It returns the milliseconds from a Duration message.
func (m *Module) jsDurationMs(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("durationMs: %s", err))
	}
	ms, err := checkedDurationDescriptor(msg, m.state.durationType.Descriptor())
	if err != nil {
		panic(m.runtime.NewTypeError("durationMs: %s", err))
	}
	return m.int64ToGoja(ms)
}

// jsAnyPack is the JS-facing implementation of pb.anyPack(msgType, msg).
// It wraps a protobuf message into a google.protobuf.Any.
func (m *Module) jsAnyPack(call goja.FunctionCall) goja.Value {
	msgTypeVal := call.Argument(0)
	messageType, err := m.extractMessageType(msgTypeVal)
	if err != nil {
		panic(m.runtime.NewTypeError("anyPack: first argument: %s", err))
	}

	msg, err := m.unwrapMessage(call.Argument(1))
	if err != nil {
		panic(m.runtime.NewTypeError("anyPack: second argument: %s", err))
	}

	// Verify message type matches.
	reflected := msg.ProtoReflect()
	messageDesc := messageType.Descriptor()
	if reflected.Descriptor() != messageDesc {
		panic(m.runtime.NewTypeError("anyPack: message type %q does not match schema %q",
			reflected.Descriptor().FullName(), messageDesc.FullName()))
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		panic(m.runtime.NewGoError(fmt.Errorf("anyPack: marshal: %w", err)))
	}
	anyDescriptor := m.state.anyType.Descriptor()
	packed := m.state.anyType.New()
	packed.Set(
		anyDescriptor.Fields().ByName("type_url"),
		protoreflect.ValueOfString("type.googleapis.com/"+string(messageDesc.FullName())),
	)
	packed.Set(
		anyDescriptor.Fields().ByName("value"),
		protoreflect.ValueOfBytes(data),
	)
	return m.wrapMessage(packed.Interface())
}

// jsAnyUnpack is the JS-facing implementation of
// pb.anyUnpack(anyMsg, msgType). It extracts a message from a
// google.protobuf.Any.
func (m *Module) jsAnyUnpack(call goja.FunctionCall) goja.Value {
	anyMsg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("anyUnpack: first argument: %s", err))
	}

	messageType, err := m.extractMessageType(call.Argument(1))
	if err != nil {
		panic(m.runtime.NewTypeError("anyUnpack: second argument: %s", err))
	}

	anyDescriptor := m.state.anyType.Descriptor()
	reflected, err := requireMessage(anyMsg, anyDescriptor)
	if err != nil {
		panic(m.runtime.NewTypeError("anyUnpack: first argument: %s", err))
	}
	typeURL := reflected.Get(anyDescriptor.Fields().ByName("type_url")).String()
	wantName := string(messageType.Descriptor().FullName())
	if !typeURLMatches(typeURL, wantName) {
		panic(m.runtime.NewTypeError("anyUnpack: type URL %q does not identify %q", typeURL, wantName))
	}
	data := reflected.Get(anyDescriptor.Fields().ByName("value")).Bytes()
	msg := messageType.New().Interface()
	if err := (proto.UnmarshalOptions{Resolver: m.typeResolver()}).Unmarshal(data, msg); err != nil {
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

	anyDescriptor := m.state.anyType.Descriptor()
	reflected, err := requireMessage(anyMsg, anyDescriptor)
	if err != nil {
		panic(m.runtime.NewTypeError("anyIs: first argument: %s", err))
	}
	typeURL := reflected.Get(anyDescriptor.Fields().ByName("type_url")).String()

	// Second argument: string type name or message type constructor.
	typeArg := call.Argument(1)
	if typeArg == nil || goja.IsUndefined(typeArg) || goja.IsNull(typeArg) {
		panic(m.runtime.NewTypeError("anyIs: second argument must be a type name or message type"))
	}

	// Try as message type constructor first.
	if messageType, typeErr := m.extractMessageType(typeArg); typeErr == nil {
		return m.runtime.ToValue(typeURLMatches(typeURL, string(messageType.Descriptor().FullName())))
	}

	// Fall back to string comparison.
	wantName := typeArg.String()
	return m.runtime.ToValue(typeURLMatches(typeURL, wantName))
}

// ---------- Internal helpers ----------

func timestampMessage(messageType protoreflect.MessageType, seconds int64, nanos int32) proto.Message {
	message := messageType.New()
	descriptor := messageType.Descriptor()
	message.Set(descriptor.Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	message.Set(descriptor.Fields().ByName("nanos"), protoreflect.ValueOfInt32(nanos))
	return message.Interface()
}

func timestampMillis(messageType protoreflect.MessageType, ms int64) proto.Message {
	seconds, nanos := timestampParts(ms)
	return timestampMessage(messageType, seconds, nanos)
}

func timestampParts(ms int64) (int64, int32) {
	seconds := ms / 1000
	nanos := (ms % 1000) * 1_000_000
	if nanos < 0 {
		seconds--
		nanos += 1_000_000_000
	}
	return seconds, int32(nanos)
}

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
	seconds, nanos := timestampParts(ms)
	msg.Set(timestampDesc.Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	msg.Set(timestampDesc.Fields().ByName("nanos"), protoreflect.ValueOfInt32(nanos))
	return msg
}

// timestampToMs extracts epoch milliseconds from a Timestamp message.
func checkedTimestampToMs(msg proto.Message) (int64, error) {
	return checkedTimestampDescriptor(msg, timestampDesc)
}

func checkedTimestampDescriptor(
	msg proto.Message,
	descriptor protoreflect.MessageDescriptor,
) (int64, error) {
	reflected, err := requireMessage(msg, descriptor)
	if err != nil {
		return 0, err
	}
	seconds := reflected.Get(descriptor.Fields().ByName("seconds")).Int()
	nanos := reflected.Get(descriptor.Fields().ByName("nanos")).Int()
	if err := (&timestamppb.Timestamp{Seconds: seconds, Nanos: int32(nanos)}).CheckValid(); err != nil {
		return 0, err
	}
	return seconds*1000 + nanos/1_000_000, nil
}

func timestampToMs(msg proto.Message) int64 {
	value, err := checkedTimestampToMs(msg)
	if err != nil {
		panic(err)
	}
	return value
}

func durationMessage(messageType protoreflect.MessageType, seconds int64, nanos int32) proto.Message {
	message := messageType.New()
	descriptor := messageType.Descriptor()
	message.Set(descriptor.Fields().ByName("seconds"), protoreflect.ValueOfInt64(seconds))
	message.Set(descriptor.Fields().ByName("nanos"), protoreflect.ValueOfInt32(nanos))
	return message.Interface()
}

func durationMillis(messageType protoreflect.MessageType, ms int64) proto.Message {
	return durationMessage(
		messageType,
		ms/1000,
		int32((ms%1000)*1_000_000),
	)
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
func checkedDurationToMs(msg proto.Message) (int64, error) {
	return checkedDurationDescriptor(msg, durationDesc)
}

func checkedDurationDescriptor(
	msg proto.Message,
	descriptor protoreflect.MessageDescriptor,
) (int64, error) {
	reflected, err := requireMessage(msg, descriptor)
	if err != nil {
		return 0, err
	}
	seconds := reflected.Get(descriptor.Fields().ByName("seconds")).Int()
	nanos := reflected.Get(descriptor.Fields().ByName("nanos")).Int()
	if err := (&durationpb.Duration{Seconds: seconds, Nanos: int32(nanos)}).CheckValid(); err != nil {
		return 0, err
	}
	return seconds*1000 + nanos/1_000_000, nil
}

func durationToMs(msg proto.Message) int64 {
	value, err := checkedDurationToMs(msg)
	if err != nil {
		panic(err)
	}
	return value
}

func validateTimestampDescriptor(msg proto.Message, descriptor protoreflect.MessageDescriptor) error {
	_, err := checkedTimestampDescriptor(msg, descriptor)
	return err
}

func validateDurationDescriptor(msg proto.Message, descriptor protoreflect.MessageDescriptor) error {
	_, err := checkedDurationDescriptor(msg, descriptor)
	return err
}

func requireMessage(msg proto.Message, descriptor protoreflect.MessageDescriptor) (protoreflect.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("expected %s, got nil", descriptor.FullName())
	}
	reflected := msg.ProtoReflect()
	if reflected.Descriptor() != descriptor {
		return nil, fmt.Errorf("expected %s, got %s", descriptor.FullName(), reflected.Descriptor().FullName())
	}
	return reflected, nil
}

func typeURLMatches(typeURL, fullName string) bool {
	slash := strings.LastIndexByte(typeURL, '/')
	if slash < 0 {
		return typeURL == fullName
	}
	return slash != len(typeURL)-1 && typeURL[slash+1:] == fullName
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
			return m.gojaToInt64(result)
		}
	}

	// Fall back to numeric value.
	return m.gojaToInt64(val)
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
