package gojagrpc

import (
	"fmt"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// snapshotMessage authenticates a JavaScript wrapper through the protobuf
// module, validates its declared RPC type, and clones it while the Goja owner is
// active. Workers therefore never observe later JavaScript mutations.
func (m *Module) snapshotMessage(value goja.Value, expected protoreflect.MessageDescriptor) (proto.Message, error) {
	message, err := m.protobuf.UnwrapMessage(value)
	if err != nil {
		return nil, err
	}
	if err := validateMessageDescriptor(message, expected); err != nil {
		return nil, err
	}
	return proto.Clone(message), nil
}

func validateMessageDescriptor(message proto.Message, expected protoreflect.MessageDescriptor) error {
	if message == nil {
		return fmt.Errorf("protobuf message is nil")
	}
	if expected == nil {
		return fmt.Errorf("expected protobuf descriptor is nil")
	}
	actual := message.ProtoReflect().Descriptor()
	if actual != expected {
		var actualName protoreflect.FullName
		if actual != nil {
			actualName = actual.FullName()
		}
		return fmt.Errorf(
			"protobuf type mismatch: got %q with non-canonical descriptor, want %q",
			actualName,
			expected.FullName(),
		)
	}
	return nil
}

// wrapMessage validates transport data before exposing it to JavaScript.
// WrapMessage is called only by the current logical adapter owner.
func (m *Module) wrapMessage(message proto.Message, expected protoreflect.MessageDescriptor) (*goja.Object, error) {
	if err := validateMessageDescriptor(message, expected); err != nil {
		return nil, err
	}
	return m.protobuf.WrapMessage(proto.Clone(message))
}
