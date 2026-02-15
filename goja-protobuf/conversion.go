package gojaprotobuf

import (
	"fmt"
	"math"
	"math/big"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	maxSafeInteger = int64(1<<53 - 1)
	minSafeInteger = -maxSafeInteger
)

// protoValueToGoja converts a protobuf [protoreflect.Value] to a
// [goja.Value], using the field descriptor to determine the
// appropriate JS type.
func (m *Module) protoValueToGoja(val protoreflect.Value, fd protoreflect.FieldDescriptor) goja.Value {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return m.runtime.ToValue(val.Bool())

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return m.runtime.ToValue(val.Int())

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return m.int64ToGoja(val.Int())

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return m.runtime.ToValue(val.Uint())

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return m.uint64ToGoja(val.Uint())

	case protoreflect.FloatKind:
		return m.runtime.ToValue(float64(val.Float()))

	case protoreflect.DoubleKind:
		return m.runtime.ToValue(val.Float())

	case protoreflect.StringKind:
		return m.runtime.ToValue(val.String())

	case protoreflect.BytesKind:
		b := val.Bytes()
		if b == nil {
			b = []byte{}
		}
		return m.newUint8Array(b)

	case protoreflect.EnumKind:
		return m.runtime.ToValue(int32(val.Enum()))

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return m.protoMessageToGoja(val.Message())

	default:
		return goja.Undefined()
	}
}

// protoMessageToGoja wraps a [protoreflect.Message] as a JS object.
func (m *Module) protoMessageToGoja(msg protoreflect.Message) goja.Value {
	dm, ok := msg.Interface().(*dynamicpb.Message)
	if ok {
		return m.wrapMessage(dm)
	}

	// Non-dynamic message: copy into a dynamicpb.Message so we have a
	// uniform wrapper.
	dm = dynamicpb.NewMessage(msg.Descriptor())
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		dm.Set(fd, v)
		return true
	})
	return m.wrapMessage(dm)
}

// int64ToGoja converts an int64 to a JS value. Values within the safe
// integer range are returned as numbers; values outside use BigInt.
func (m *Module) int64ToGoja(v int64) goja.Value {
	if v >= minSafeInteger && v <= maxSafeInteger {
		return m.runtime.ToValue(v)
	}
	return m.runtime.ToValue(new(big.Int).SetInt64(v))
}

// uint64ToGoja converts a uint64 to a JS value. Values within the safe
// integer range are returned as numbers; values outside use BigInt.
func (m *Module) uint64ToGoja(v uint64) goja.Value {
	if v <= uint64(maxSafeInteger) {
		return m.runtime.ToValue(v)
	}
	return m.runtime.ToValue(new(big.Int).SetUint64(v))
}

// gojaToProtoValue converts a [goja.Value] to a [protoreflect.Value],
// using the field descriptor to determine the target proto type. This
// handles scalar fields only; for repeated/map fields, use the
// specialised setters.
func (m *Module) gojaToProtoValue(val goja.Value, fd protoreflect.FieldDescriptor) (protoreflect.Value, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		// For message fields, fd.Default() returns an invalid
		// protoreflect.Value that panics on access. Return a zero
		// value error instead so callers can skip or handle it.
		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			return protoreflect.Value{}, fmt.Errorf("null value for message field %s", fd.Name())
		}
		return fd.Default(), nil
	}

	switch fd.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(val.ToBoolean()), nil

	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		v, err := m.gojaToInt64(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		if v < math.MinInt32 || v > math.MaxInt32 {
			return protoreflect.Value{}, fmt.Errorf("value %d overflows int32", v)
		}
		return protoreflect.ValueOfInt32(int32(v)), nil

	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		v, err := m.gojaToInt64(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfInt64(v), nil

	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		v, err := m.gojaToUint64(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		if v > math.MaxUint32 {
			return protoreflect.Value{}, fmt.Errorf("value %d overflows uint32", v)
		}
		return protoreflect.ValueOfUint32(uint32(v)), nil

	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		v, err := m.gojaToUint64(val)
		if err != nil {
			return protoreflect.Value{}, err
		}
		return protoreflect.ValueOfUint64(v), nil

	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(float32(val.ToFloat())), nil

	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(val.ToFloat()), nil

	case protoreflect.StringKind:
		return protoreflect.ValueOfString(val.String()), nil

	case protoreflect.BytesKind:
		b, err := m.extractBytes(val)
		if err != nil {
			return protoreflect.Value{}, fmt.Errorf("field %s: %w", fd.Name(), err)
		}
		return protoreflect.ValueOfBytes(b), nil

	case protoreflect.EnumKind:
		return m.gojaToProtoEnum(val, fd)

	case protoreflect.MessageKind, protoreflect.GroupKind:
		return m.gojaToProtoMessage(val, fd)

	default:
		return protoreflect.Value{}, fmt.Errorf("unsupported field kind: %s", fd.Kind())
	}
}

// gojaToInt64 converts a goja Value to int64, handling number and BigInt.
func (m *Module) gojaToInt64(val goja.Value) (int64, error) {
	exported := val.Export()
	switch v := exported.(type) {
	case int64:
		return v, nil
	case *big.Int:
		if !v.IsInt64() {
			return 0, fmt.Errorf("BigInt value %s overflows int64", v.String())
		}
		return v.Int64(), nil
	default:
		return val.ToInteger(), nil
	}
}

// gojaToUint64 converts a goja Value to uint64, handling number and BigInt.
func (m *Module) gojaToUint64(val goja.Value) (uint64, error) {
	exported := val.Export()
	switch v := exported.(type) {
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("negative value %d for unsigned field", v)
		}
		return uint64(v), nil
	case *big.Int:
		if v.Sign() < 0 {
			return 0, fmt.Errorf("negative BigInt for unsigned field")
		}
		if !v.IsUint64() {
			return 0, fmt.Errorf("BigInt value %s overflows uint64", v.String())
		}
		return v.Uint64(), nil
	default:
		i := val.ToInteger()
		if i < 0 {
			return 0, fmt.Errorf("negative value %d for unsigned field", i)
		}
		return uint64(i), nil
	}
}

// gojaToProtoEnum converts a goja Value to a protobuf enum Value.
// Accepts both numeric values and string enum names.
func (m *Module) gojaToProtoEnum(val goja.Value, fd protoreflect.FieldDescriptor) (protoreflect.Value, error) {
	exported := val.Export()

	// Accept string name.
	if s, ok := exported.(string); ok {
		enumDesc := fd.Enum()
		evd := enumDesc.Values().ByName(protoreflect.Name(s))
		if evd == nil {
			return protoreflect.Value{}, fmt.Errorf("unknown enum value name %q for %s", s, enumDesc.FullName())
		}
		return protoreflect.ValueOfEnum(evd.Number()), nil
	}

	// Accept number.
	v, err := m.gojaToInt64(val)
	if err != nil {
		return protoreflect.Value{}, err
	}
	if v < math.MinInt32 || v > math.MaxInt32 {
		return protoreflect.Value{}, fmt.Errorf("enum value %d overflows int32", v)
	}
	return protoreflect.ValueOfEnum(protoreflect.EnumNumber(int32(v))), nil
}

// gojaToProtoMessage converts a goja Value to a protobuf message Value.
// Accepts wrapped message objects or plain JS objects.
func (m *Module) gojaToProtoMessage(val goja.Value, fd protoreflect.FieldDescriptor) (protoreflect.Value, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return protoreflect.Value{}, fmt.Errorf("null value for message field %s", fd.Name())
	}

	// Check for wrapped message.
	if wrapper, err := m.unwrapMessage(val); err == nil {
		got := wrapper.ProtoReflect().Descriptor().FullName()
		want := fd.Message().FullName()
		if got != want {
			return protoreflect.Value{}, fmt.Errorf(
				"message type mismatch for field %s: expected %s, got %s",
				fd.Name(), want, got)
		}
		return protoreflect.ValueOfMessage(wrapper.ProtoReflect()), nil
	}

	// Plain JS object — convert field by field.
	obj := val.ToObject(m.runtime)
	if obj == nil {
		return protoreflect.Value{}, fmt.Errorf("expected message or object for field %s", fd.Name())
	}

	msgDesc := fd.Message()
	msg, err := m.jsObjectToMessage(obj, msgDesc)
	if err != nil {
		return protoreflect.Value{}, err
	}
	return protoreflect.ValueOfMessage(msg.ProtoReflect()), nil
}

// jsObjectToMessage converts a plain JS object to a [dynamicpb.Message]
// by iterating the message descriptor's fields and extracting matching
// properties from the JS object.
func (m *Module) jsObjectToMessage(obj *goja.Object, msgDesc protoreflect.MessageDescriptor) (*dynamicpb.Message, error) {
	msg := dynamicpb.NewMessage(msgDesc)
	fields := msgDesc.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		fieldName := string(fd.Name())
		fieldVal := obj.Get(fieldName)
		if fieldVal == nil || goja.IsUndefined(fieldVal) || goja.IsNull(fieldVal) {
			continue
		}
		if fd.IsList() {
			if err := m.setRepeatedFromGoja(msg, fd, fieldVal); err != nil {
				return nil, err
			}
		} else if fd.IsMap() {
			if err := m.setMapFromGoja(msg, fd, fieldVal); err != nil {
				return nil, err
			}
		} else {
			pv, err := m.gojaToProtoValue(fieldVal, fd)
			if err != nil {
				return nil, err
			}
			msg.Set(fd, pv)
		}
	}
	return msg, nil
}

// gojaToProtoMapKey converts a goja Value to a [protoreflect.MapKey].
func (m *Module) gojaToProtoMapKey(val goja.Value, fd protoreflect.FieldDescriptor) (protoreflect.MapKey, error) {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		// When keys come from a plain JS object, they arrive as
		// strings ("true"/"false"). JS truthiness would convert
		// "false" → true, so we must parse string values explicitly.
		if s, ok := val.Export().(string); ok {
			return protoreflect.ValueOfBool(s == "true").MapKey(), nil
		}
		return protoreflect.ValueOfBool(val.ToBoolean()).MapKey(), nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		v, err := m.gojaToInt64(val)
		if err != nil {
			return protoreflect.MapKey{}, err
		}
		if v < math.MinInt32 || v > math.MaxInt32 {
			return protoreflect.MapKey{}, fmt.Errorf("map key value %d overflows int32", v)
		}
		return protoreflect.ValueOfInt32(int32(v)).MapKey(), nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		v, err := m.gojaToInt64(val)
		if err != nil {
			return protoreflect.MapKey{}, err
		}
		return protoreflect.ValueOfInt64(v).MapKey(), nil
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		v, err := m.gojaToUint64(val)
		if err != nil {
			return protoreflect.MapKey{}, err
		}
		if v > math.MaxUint32 {
			return protoreflect.MapKey{}, fmt.Errorf("map key value %d overflows uint32", v)
		}
		return protoreflect.ValueOfUint32(uint32(v)).MapKey(), nil
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		v, err := m.gojaToUint64(val)
		if err != nil {
			return protoreflect.MapKey{}, err
		}
		return protoreflect.ValueOfUint64(v).MapKey(), nil
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(val.String()).MapKey(), nil
	default:
		return protoreflect.MapKey{}, fmt.Errorf("unsupported map key kind: %s", fd.Kind())
	}
}

// setRepeatedFromGoja sets a repeated field on msg from a JS array value.
func (m *Module) setRepeatedFromGoja(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor, val goja.Value) error {
	obj := val.ToObject(m.runtime)
	if obj == nil {
		return fmt.Errorf("expected array for repeated field %s", fd.Name())
	}

	lenVal := obj.Get("length")
	if lenVal == nil || goja.IsUndefined(lenVal) {
		return fmt.Errorf("expected array for repeated field %s", fd.Name())
	}

	length := int(lenVal.ToInteger())
	list := msg.Mutable(fd).List()
	for i := range length {
		elem := obj.Get(fmt.Sprintf("%d", i))
		if elem == nil || goja.IsUndefined(elem) || goja.IsNull(elem) {
			continue
		}
		pv, err := m.gojaToProtoValue(elem, fd)
		if err != nil {
			return fmt.Errorf("repeated field %s[%d]: %w", fd.Name(), i, err)
		}
		list.Append(pv)
	}

	return nil
}

// setMapFromGoja sets a map field on msg from a JS object or Map.
func (m *Module) setMapFromGoja(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor, val goja.Value) error {
	obj := val.ToObject(m.runtime)
	if obj == nil {
		return fmt.Errorf("expected object for map field %s", fd.Name())
	}

	keyDesc := fd.MapKey()
	valueDesc := fd.MapValue()
	protoMap := msg.Mutable(fd).Map()

	// Check whether the value is a JS Map with an entries() method.
	entriesVal := obj.Get("entries")
	if entriesVal != nil && !goja.IsUndefined(entriesVal) {
		if entriesFn, ok := goja.AssertFunction(entriesVal); ok {
			iterVal, callErr := entriesFn(obj)
			if callErr == nil {
				iterObj := iterVal.ToObject(m.runtime)
				if nextFn, nextOk := goja.AssertFunction(iterObj.Get("next")); nextOk {
					for {
						result, err := nextFn(iterObj)
						if err != nil {
							return fmt.Errorf("map field %s iterator: %w", fd.Name(), err)
						}
						resObj := result.ToObject(m.runtime)
						if resObj.Get("done").ToBoolean() {
							break
						}
						entry := resObj.Get("value").ToObject(m.runtime)
						k := entry.Get("0")
						v := entry.Get("1")

						// Skip null/undefined values.
						if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
							continue
						}

						mk, err := m.gojaToProtoMapKey(k, keyDesc)
						if err != nil {
							return fmt.Errorf("map field %s key: %w", fd.Name(), err)
						}
						mv, err := m.gojaToProtoValue(v, valueDesc)
						if err != nil {
							return fmt.Errorf("map field %s value: %w", fd.Name(), err)
						}
						protoMap.Set(mk, mv)
					}
					return nil
				}
			}
		}
	}

	// Fall back to plain object: iterate own property keys.
	keys := obj.Keys()
	for _, key := range keys {
		v := obj.Get(key)
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			continue
		}
		mk, err := m.gojaToProtoMapKey(m.runtime.ToValue(key), keyDesc)
		if err != nil {
			return fmt.Errorf("map field %s key %q: %w", fd.Name(), key, err)
		}
		mv, err := m.gojaToProtoValue(v, valueDesc)
		if err != nil {
			return fmt.Errorf("map field %s value for key %q: %w", fd.Name(), key, err)
		}
		protoMap.Set(mk, mv)
	}

	return nil
}

// mapKeyToGoja converts a [protoreflect.MapKey] to a [goja.Value].
func (m *Module) mapKeyToGoja(mk protoreflect.MapKey, fd protoreflect.FieldDescriptor) goja.Value {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return m.runtime.ToValue(mk.Bool())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return m.runtime.ToValue(mk.Int())
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return m.int64ToGoja(mk.Int())
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return m.runtime.ToValue(mk.Uint())
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return m.uint64ToGoja(mk.Uint())
	case protoreflect.StringKind:
		return m.runtime.ToValue(mk.String())
	default:
		return m.runtime.ToValue(mk.String())
	}
}
