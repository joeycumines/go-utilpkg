package gojaprotobuf

import (
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsLoadDescriptorSet implements pb.loadDescriptorSet(bytes).
func (m *Module) jsLoadDescriptorSet(call goja.FunctionCall) goja.Value {
	data, err := m.extractBytes(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}
	names, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}
	return m.stringArray(names)
}

// jsLoadFileDescriptorProto implements pb.loadFileDescriptorProto(bytes).
func (m *Module) jsLoadFileDescriptorProto(call goja.FunctionCall) goja.Value {
	data, err := m.extractBytes(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}
	names, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}
	return m.stringArray(names)
}

func (m *Module) stringArray(values []string) *goja.Object {
	array := m.runtime.NewArray()
	for index, value := range values {
		if err := array.Set(strconv.Itoa(index), value); err != nil {
			panic(m.runtime.NewGoError(err))
		}
	}
	return array
}

// loadDescriptorSetBytes atomically installs an entire descriptor graph.
// Input order is irrelevant. An exact duplicate file is idempotent, while a
// reused path or symbol with different content is rejected.
func (m *Module) loadDescriptorSetBytes(data []byte) ([]string, error) {
	set := new(descriptorpb.FileDescriptorSet)
	if err := proto.Unmarshal(data, set); err != nil {
		return nil, fmt.Errorf("decode descriptor set: %w", err)
	}
	return m.installFileProtos(set.GetFile())
}

func (m *Module) loadFileDescriptorProtoBytes(data []byte) ([]string, error) {
	file := new(descriptorpb.FileDescriptorProto)
	if err := proto.Unmarshal(data, file); err != nil {
		return nil, fmt.Errorf("decode file descriptor: %w", err)
	}
	return m.installFileProtos([]*descriptorpb.FileDescriptorProto{file})
}

func (m *Module) installFileProtos(input []*descriptorpb.FileDescriptorProto) ([]string, error) {
	incoming := make(map[string]*descriptorpb.FileDescriptorProto, len(input))
	for index, file := range input {
		if file == nil {
			return nil, fmt.Errorf("descriptor file %d is nil", index)
		}
		name := file.GetName()
		if name == "" {
			return nil, fmt.Errorf("descriptor file %d has no name", index)
		}
		cloned := proto.Clone(file).(*descriptorpb.FileDescriptorProto)
		if previous, ok := incoming[name]; ok {
			if !proto.Equal(previous, cloned) {
				return nil, fmt.Errorf("descriptor set contains conflicting definitions for %q", name)
			}
			continue
		}
		incoming[name] = cloned
	}

	m.state.mu.Lock()
	defer m.state.mu.Unlock()

	stagedFiles, err := cloneFiles(m.state.localFiles)
	if err != nil {
		return nil, fmt.Errorf("stage file registry: %w", err)
	}
	stagedTypes, err := cloneTypes(m.state.localTypes)
	if err != nil {
		return nil, fmt.Errorf("stage type registry: %w", err)
	}
	stagedProtos := make(map[string]*descriptorpb.FileDescriptorProto, len(m.state.localProtos)+len(incoming))
	maps.Copy(stagedProtos, m.state.localProtos)

	pending := make(map[string]*descriptorpb.FileDescriptorProto, len(incoming))
	for name, file := range incoming {
		if existing, ok := stagedProtos[name]; ok {
			if !proto.Equal(existing, file) {
				return nil, fmt.Errorf("file %q is already registered with different content", name)
			}
			continue
		}
		if base, ok := m.state.baseGraph.files[name]; ok {
			if !proto.Equal(protodesc.ToFileDescriptorProto(base), file) {
				return nil, fmt.Errorf("file %q conflicts with the base registry", name)
			}
			continue
		}
		pending[name] = file
	}

	resolver := &combinedFileResolver{
		local:  stagedFiles,
		global: m.state.baseFiles,
		graph:  m.state.baseGraph,
	}
	var names []string
	for len(pending) != 0 {
		paths := sortedProtoPaths(pending)
		progress := false
		failures := make(map[string]error, len(paths))
		for _, path := range paths {
			file, buildErr := protodesc.NewFile(pending[path], resolver)
			if buildErr != nil {
				failures[path] = buildErr
				continue
			}
			fileGraph, graphErr := descriptorFileGraph(file)
			if graphErr != nil {
				return nil, fmt.Errorf("validate file %q: %w", path, graphErr)
			}
			if conflictErr := m.state.baseGraph.compatible(fileGraph); conflictErr != nil {
				return nil, conflictErr
			}
			if registerErr := stagedFiles.RegisterFile(file); registerErr != nil {
				return nil, fmt.Errorf("register file %q: %w", path, registerErr)
			}
			fileNames, registerErr := registerFileTypes(stagedTypes, file)
			if registerErr != nil {
				return nil, fmt.Errorf("register types from %q: %w", path, registerErr)
			}
			names = append(names, fileNames...)
			stagedProtos[path] = pending[path]
			delete(pending, path)
			progress = true
		}
		if progress {
			continue
		}
		details := make([]string, 0, len(paths))
		for _, path := range paths {
			details = append(details, fmt.Sprintf("%s: %v", path, failures[path]))
		}
		return nil, fmt.Errorf("descriptor graph cannot be resolved: %s", strings.Join(details, "; "))
	}

	stagedGraph, err := buildDescriptorGraph(stagedTypes, stagedFiles)
	if err != nil {
		return nil, fmt.Errorf("validate staged descriptor graph: %w", err)
	}
	if err := m.state.baseGraph.compatible(stagedGraph); err != nil {
		return nil, fmt.Errorf("validate staged descriptor graph: %w", err)
	}
	sort.Strings(names)
	m.state.localFiles = stagedFiles
	m.state.localTypes = stagedTypes
	m.state.localProtos = stagedProtos
	return names, nil
}

func sortedProtoPaths(files map[string]*descriptorpb.FileDescriptorProto) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func cloneFiles(source *protoregistry.Files) (*protoregistry.Files, error) {
	files := collectFileRoots(source)
	target := new(protoregistry.Files)
	for _, file := range files {
		if err := target.RegisterFile(file); err != nil {
			return nil, err
		}
	}
	return target, nil
}

func cloneTypes(source *protoregistry.Types) (*protoregistry.Types, error) {
	target := new(protoregistry.Types)
	var messages []protoreflect.MessageType
	source.RangeMessages(func(message protoreflect.MessageType) bool {
		messages = append(messages, message)
		return true
	})
	sort.Slice(messages, func(left, right int) bool {
		return messages[left].Descriptor().FullName() <
			messages[right].Descriptor().FullName()
	})
	for _, message := range messages {
		if err := target.RegisterMessage(message); err != nil {
			return nil, err
		}
	}
	var enums []protoreflect.EnumType
	source.RangeEnums(func(enum protoreflect.EnumType) bool {
		enums = append(enums, enum)
		return true
	})
	sort.Slice(enums, func(left, right int) bool {
		return enums[left].Descriptor().FullName() <
			enums[right].Descriptor().FullName()
	})
	for _, enum := range enums {
		if err := target.RegisterEnum(enum); err != nil {
			return nil, err
		}
	}
	var extensions []protoreflect.ExtensionType
	source.RangeExtensions(func(extension protoreflect.ExtensionType) bool {
		extensions = append(extensions, extension)
		return true
	})
	sort.Slice(extensions, func(left, right int) bool {
		return extensions[left].TypeDescriptor().FullName() <
			extensions[right].TypeDescriptor().FullName()
	})
	for _, extension := range extensions {
		if err := target.RegisterExtension(extension); err != nil {
			return nil, err
		}
	}
	return target, nil
}

func registerFileTypes(types *protoregistry.Types, file protoreflect.FileDescriptor) ([]string, error) {
	var names []string
	if err := registerMessages(types, file.Messages(), &names); err != nil {
		return nil, err
	}
	if err := registerEnums(types, file.Enums(), &names); err != nil {
		return nil, err
	}
	if err := registerExtensions(types, file.Extensions(), &names); err != nil {
		return nil, err
	}
	return names, nil
}

func registerMessages(types *protoregistry.Types, messages protoreflect.MessageDescriptors, names *[]string) error {
	for index := 0; index < messages.Len(); index++ {
		message := messages.Get(index)
		if err := types.RegisterMessage(dynamicpb.NewMessageType(message)); err != nil {
			return err
		}
		*names = append(*names, string(message.FullName()))
		if err := registerMessages(types, message.Messages(), names); err != nil {
			return err
		}
		if err := registerEnums(types, message.Enums(), names); err != nil {
			return err
		}
		if err := registerExtensions(types, message.Extensions(), names); err != nil {
			return err
		}
	}
	return nil
}

func registerEnums(types *protoregistry.Types, enums protoreflect.EnumDescriptors, names *[]string) error {
	for index := 0; index < enums.Len(); index++ {
		enum := enums.Get(index)
		if err := types.RegisterEnum(dynamicpb.NewEnumType(enum)); err != nil {
			return err
		}
		*names = append(*names, string(enum.FullName()))
	}
	return nil
}

func registerExtensions(types *protoregistry.Types, extensions protoreflect.ExtensionDescriptors, names *[]string) error {
	for index := 0; index < extensions.Len(); index++ {
		extension := extensions.Get(index)
		if err := types.RegisterExtension(dynamicpb.NewExtensionType(extension)); err != nil {
			return err
		}
		*names = append(*names, string(extension.FullName()))
	}
	return nil
}

func registeredEnumValues(
	types *protoregistry.Types,
	name protoreflect.FullName,
) []protoreflect.EnumValueDescriptor {
	var values []protoreflect.EnumValueDescriptor
	types.RangeEnums(func(enum protoreflect.EnumType) bool {
		enumValues := enum.Descriptor().Values()
		for index := 0; index < enumValues.Len(); index++ {
			value := enumValues.Get(index)
			if value.FullName() == name {
				values = append(values, value)
			}
		}
		return true
	})
	return values
}
