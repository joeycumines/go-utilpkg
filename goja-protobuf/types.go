package gojaprotobuf

import (
	"strconv"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsMessageType is the JS-facing implementation of
// pb.messageType(fullName). It returns a constructor function that,
// when called with new, creates a new wrapped [dynamicpb.Message].
func (m *Module) jsMessageType(call goja.FunctionCall) goja.Value {
	fullName := call.Argument(0).String()

	msgDesc, err := m.findMessageDescriptor(protoreflect.FullName(fullName))
	if err != nil {
		panic(m.runtime.NewTypeError("message type %q not found", fullName))
	}

	// Build a constructor function.
	ctorFn := func(call goja.ConstructorCall) *goja.Object {
		msg := dynamicpb.NewMessage(msgDesc)
		return m.wrapMessage(msg)
	}

	ctorVal := m.runtime.ToValue(ctorFn)
	ctorObj := ctorVal.ToObject(m.runtime)

	// Store the descriptor on the constructor so that decode/fromJSON
	// can retrieve it.
	_ = ctorObj.Set("_pbMsgDesc", &messageDescHolder{desc: msgDesc})

	// Store the fully-qualified name for debugging.
	_ = ctorObj.Set("typeName", fullName)

	return ctorVal
}

// jsEnumType is the JS-facing implementation of pb.enumType(fullName).
// It returns a frozen object mapping name→number and number→name.
func (m *Module) jsEnumType(call goja.FunctionCall) goja.Value {
	fullName := call.Argument(0).String()

	enumDesc, err := m.findEnumDescriptor(protoreflect.FullName(fullName))
	if err != nil {
		panic(m.runtime.NewTypeError("enum type %q not found", fullName))
	}

	obj := m.runtime.NewObject()
	values := enumDesc.Values()
	for i := 0; i < values.Len(); i++ {
		evd := values.Get(i)
		name := string(evd.Name())
		number := int32(evd.Number())

		// name → number
		_ = obj.Set(name, number)
		// number → name (as string key, since JS object keys are strings)
		_ = obj.Set(strconv.Itoa(int(number)), name)
	}

	// Freeze the object to prevent accidental mutation.
	freezeVal := m.runtime.Get("Object").ToObject(m.runtime).Get("freeze")
	if freezeFn, ok := goja.AssertFunction(freezeVal); ok {
		_, _ = freezeFn(goja.Undefined(), obj)
	}
	return obj
}

// findMessageDescriptor looks up a message descriptor first in
// localTypes, then in the configured resolver.
func (m *Module) findMessageDescriptor(fullName protoreflect.FullName) (protoreflect.MessageDescriptor, error) {
	mt, err := m.localTypes.FindMessageByName(fullName)
	if err == nil {
		return mt.Descriptor(), nil
	}
	mt, err = m.resolver.FindMessageByName(fullName)
	if err == nil {
		return mt.Descriptor(), nil
	}
	return nil, err
}

// findEnumDescriptor looks up an enum descriptor first in localTypes,
// then in the configured resolver.
func (m *Module) findEnumDescriptor(fullName protoreflect.FullName) (protoreflect.EnumDescriptor, error) {
	et, err := m.localTypes.FindEnumByName(fullName)
	if err == nil {
		return et.Descriptor(), nil
	}
	et, err = m.resolver.FindEnumByName(fullName)
	if err == nil {
		return et.Descriptor(), nil
	}
	return nil, err
}
