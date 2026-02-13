package gojaprotobuf

import (
	"strconv"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsLoadDescriptorSet is the JS-facing implementation of
// pb.loadDescriptorSet(bytes). It accepts serialized
// [descriptorpb.FileDescriptorSet] bytes and registers all contained
// message and enum types into the module's local registries. Returns
// an array of fully-qualified type names that were registered.
func (m *Module) jsLoadDescriptorSet(call goja.FunctionCall) goja.Value {
	data, err := m.extractBytes(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	names, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	arr := m.runtime.NewArray()
	for i, name := range names {
		_ = arr.Set(strconv.Itoa(i), m.runtime.ToValue(name))
	}
	return arr
}

// jsLoadFileDescriptorProto is the JS-facing implementation of
// pb.loadFileDescriptorProto(bytes). It accepts serialized
// [descriptorpb.FileDescriptorProto] bytes and registers the
// contained types.
func (m *Module) jsLoadFileDescriptorProto(call goja.FunctionCall) goja.Value {
	data, err := m.extractBytes(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	names, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	arr := m.runtime.NewArray()
	for i, name := range names {
		_ = arr.Set(strconv.Itoa(i), m.runtime.ToValue(name))
	}
	return arr
}

// loadDescriptorSetBytes parses a serialised FileDescriptorSet and
// registers all contained types into the module's local registries.
func (m *Module) loadDescriptorSetBytes(data []byte) ([]string, error) {
	fds := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(data, fds); err != nil {
		return nil, err
	}

	resolver := m.fileResolver()

	var names []string
	for _, fdp := range fds.GetFile() {
		// Skip files that are already registered.
		if _, err := m.localFiles.FindFileByPath(fdp.GetName()); err == nil {
			continue
		}

		fd, err := protodesc.NewFile(fdp, resolver)
		if err != nil {
			return nil, err
		}
		if regErr := m.localFiles.RegisterFile(fd); regErr != nil {
			// The FindFileByPath check above should have caught
			// already-registered files, but a race-free duplicate
			// is possible. Treat any RegisterFile error as
			// non-fatal to be defensive.
			continue
		}
		names = append(names, m.registerFileTypes(fd)...)
	}
	return names, nil
}

// loadFileDescriptorProtoBytes parses a single serialised
// FileDescriptorProto and registers its types.
func (m *Module) loadFileDescriptorProtoBytes(data []byte) ([]string, error) {
	fdp := new(descriptorpb.FileDescriptorProto)
	if err := proto.Unmarshal(data, fdp); err != nil {
		return nil, err
	}

	// Check if already registered.
	if _, err := m.localFiles.FindFileByPath(fdp.GetName()); err == nil {
		return nil, nil
	}

	resolver := m.fileResolver()
	fd, err := protodesc.NewFile(fdp, resolver)
	if err != nil {
		return nil, err
	}
	if regErr := m.localFiles.RegisterFile(fd); regErr != nil {
		return nil, nil
	}
	return m.registerFileTypes(fd), nil
}

// registerFileTypes registers all message and enum types from a file
// descriptor into the module's localTypes. Returns the list of
// fully-qualified names that were registered.
func (m *Module) registerFileTypes(fd protoreflect.FileDescriptor) []string {
	var names []string
	names = append(names, m.registerMessageTypes(fd.Messages())...)
	names = append(names, m.registerEnumTypes(fd.Enums())...)
	return names
}

// registerMessageTypes recursively registers message types.
func (m *Module) registerMessageTypes(msgs protoreflect.MessageDescriptors) []string {
	var names []string
	for i := 0; i < msgs.Len(); i++ {
		md := msgs.Get(i)
		mt := dynamicpb.NewMessageType(md)
		if err := m.localTypes.RegisterMessage(mt); err == nil {
			names = append(names, string(md.FullName()))
		}
		// Recurse into nested messages.
		names = append(names, m.registerMessageTypes(md.Messages())...)
		// Register nested enums.
		names = append(names, m.registerEnumTypes(md.Enums())...)
	}
	return names
}

// registerEnumTypes registers enum types.
func (m *Module) registerEnumTypes(enums protoreflect.EnumDescriptors) []string {
	var names []string
	for i := 0; i < enums.Len(); i++ {
		ed := enums.Get(i)
		et := dynamicpb.NewEnumType(ed)
		if err := m.localTypes.RegisterEnum(et); err == nil {
			names = append(names, string(ed.FullName()))
		}
	}
	return names
}
