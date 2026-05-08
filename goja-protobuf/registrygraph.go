package gojaprotobuf

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

type extensionNumberKey struct {
	message protoreflect.FullName
	number  protoreflect.FieldNumber
}

// descriptorGraph is an immutable identity index after construction.
// Registry membership remains separate: the graph also includes declarations
// and dependencies reachable from registered roots.
type descriptorGraph struct {
	files      map[string]protoreflect.FileDescriptor
	packages   map[protoreflect.FullName]struct{}
	symbols    map[protoreflect.FullName]protoreflect.Descriptor
	extensions map[extensionNumberKey]protoreflect.FieldDescriptor
}

func newDescriptorGraph() *descriptorGraph {
	return &descriptorGraph{
		files:      make(map[string]protoreflect.FileDescriptor),
		packages:   make(map[protoreflect.FullName]struct{}),
		symbols:    make(map[protoreflect.FullName]protoreflect.Descriptor),
		extensions: make(map[extensionNumberKey]protoreflect.FieldDescriptor),
	}
}

func buildDescriptorGraph(
	types *protoregistry.Types,
	files *protoregistry.Files,
) (*descriptorGraph, error) {
	roots := collectTypeRoots(types)
	fileRoots := collectFileRoots(files)
	graph := newDescriptorGraph()
	for _, root := range roots {
		if err := graph.addFile(root.ParentFile()); err != nil {
			return nil, err
		}
		canonical, ok := graph.symbols[root.FullName()]
		if !ok || canonical != root {
			return nil, fmt.Errorf(
				"registered type %q is not its reachable descriptor identity",
				root.FullName(),
			)
		}
	}
	for _, file := range fileRoots {
		if err := graph.addFile(file); err != nil {
			return nil, err
		}
	}
	return graph, nil
}

func materializeGraphTypes(
	types *protoregistry.Types,
	graph *descriptorGraph,
) error {
	names := make([]protoreflect.FullName, 0, len(graph.symbols))
	for name := range graph.symbols {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		descriptor, ok := graph.symbols[name].(protoreflect.MessageDescriptor)
		if !ok {
			continue
		}
		messageType, err := types.FindMessageByName(name)
		switch {
		case err == nil:
			if messageType.Descriptor() != descriptor {
				return fmt.Errorf(
					"message type %q has divergent descriptor identity",
					name,
				)
			}
		case errors.Is(err, protoregistry.NotFound):
			if err := types.RegisterMessage(
				dynamicpb.NewMessageType(descriptor),
			); err != nil {
				return fmt.Errorf("register message type %q: %w", name, err)
			}
		default:
			return fmt.Errorf("resolve message type %q: %w", name, err)
		}
	}
	for _, name := range names {
		descriptor, ok := graph.symbols[name].(protoreflect.EnumDescriptor)
		if !ok {
			continue
		}
		enumType, err := types.FindEnumByName(name)
		switch {
		case err == nil:
			if enumType.Descriptor() != descriptor {
				return fmt.Errorf(
					"enum type %q has divergent descriptor identity",
					name,
				)
			}
		case errors.Is(err, protoregistry.NotFound):
			if err := types.RegisterEnum(
				dynamicpb.NewEnumType(descriptor),
			); err != nil {
				return fmt.Errorf("register enum type %q: %w", name, err)
			}
		default:
			return fmt.Errorf("resolve enum type %q: %w", name, err)
		}
	}
	for _, name := range names {
		descriptor, ok := graph.symbols[name].(protoreflect.FieldDescriptor)
		if !ok || !descriptor.IsExtension() {
			continue
		}
		extensionType, err := types.FindExtensionByName(name)
		switch {
		case err == nil:
			if extensionType.TypeDescriptor().Descriptor() != descriptor {
				return fmt.Errorf(
					"extension type %q has divergent descriptor identity",
					name,
				)
			}
		case errors.Is(err, protoregistry.NotFound):
			if err := types.RegisterExtension(
				dynamicpb.NewExtensionType(descriptor),
			); err != nil {
				return fmt.Errorf("register extension type %q: %w", name, err)
			}
		default:
			return fmt.Errorf("resolve extension type %q: %w", name, err)
		}
	}
	return nil
}

func collectTypeRoots(types *protoregistry.Types) []protoreflect.Descriptor {
	var roots []protoreflect.Descriptor
	types.RangeMessages(func(message protoreflect.MessageType) bool {
		roots = append(roots, message.Descriptor())
		return true
	})
	types.RangeEnums(func(enum protoreflect.EnumType) bool {
		roots = append(roots, enum.Descriptor())
		return true
	})
	types.RangeExtensions(func(extension protoreflect.ExtensionType) bool {
		roots = append(roots, extension.TypeDescriptor().Descriptor())
		return true
	})
	sort.Slice(roots, func(left, right int) bool {
		leftName := roots[left].FullName()
		rightName := roots[right].FullName()
		if leftName != rightName {
			return leftName < rightName
		}
		return roots[left].ParentFile().Path() < roots[right].ParentFile().Path()
	})
	return roots
}

func collectFileRoots(files *protoregistry.Files) []protoreflect.FileDescriptor {
	var roots []protoreflect.FileDescriptor
	files.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		roots = append(roots, file)
		return true
	})
	sort.Slice(roots, func(left, right int) bool {
		return roots[left].Path() < roots[right].Path()
	})
	return roots
}

func (g *descriptorGraph) addFile(file protoreflect.FileDescriptor) error {
	if file == nil {
		return fmt.Errorf("descriptor graph contains a nil file")
	}
	path := file.Path()
	if existing, ok := g.files[path]; ok {
		if existing != file {
			return fmt.Errorf(
				"file %q has divergent descriptor identities",
				path,
			)
		}
		return nil
	}
	g.files[path] = file
	if err := g.addPackage(file.Package()); err != nil {
		return fmt.Errorf("file %q: %w", path, err)
	}
	imports := file.Imports()
	for index := 0; index < imports.Len(); index++ {
		if err := g.addFile(imports.Get(index).FileDescriptor); err != nil {
			return fmt.Errorf("file %q import: %w", path, err)
		}
	}
	var graphErr error
	walkDescriptors(file, func(descriptor protoreflect.Descriptor) bool {
		if err := g.addSymbol(descriptor); err != nil {
			graphErr = fmt.Errorf("file %q: %w", path, err)
			return false
		}
		if err := g.addSemanticFiles(descriptor); err != nil {
			graphErr = fmt.Errorf("file %q: %w", path, err)
			return false
		}
		return true
	})
	return graphErr
}

func (g *descriptorGraph) addPackage(name protoreflect.FullName) error {
	for name != "" {
		if symbol, ok := g.symbols[name]; ok {
			return fmt.Errorf(
				"package %q conflicts with symbol %q",
				name,
				symbol.FullName(),
			)
		}
		g.packages[name] = struct{}{}
		value := string(name)
		index := strings.LastIndexByte(value, '.')
		if index < 0 {
			break
		}
		name = protoreflect.FullName(value[:index])
	}
	return nil
}

func (g *descriptorGraph) addSymbol(descriptor protoreflect.Descriptor) error {
	name := descriptor.FullName()
	if _, ok := g.packages[name]; ok {
		return fmt.Errorf("symbol %q conflicts with a package", name)
	}
	if existing, ok := g.symbols[name]; ok {
		if existing != descriptor {
			return fmt.Errorf("symbol %q has divergent descriptor identities", name)
		}
		return nil
	}
	g.symbols[name] = descriptor
	field, ok := descriptor.(protoreflect.FieldDescriptor)
	if !ok || !field.IsExtension() {
		return nil
	}
	message := field.ContainingMessage()
	if message == nil {
		return fmt.Errorf("extension %q has no containing message", name)
	}
	key := extensionNumberKey{message: message.FullName(), number: field.Number()}
	if existing, ok := g.extensions[key]; ok && existing != field {
		return fmt.Errorf(
			"extension %q conflicts with %q at %s field number %d",
			name,
			existing.FullName(),
			key.message,
			key.number,
		)
	}
	g.extensions[key] = field
	return nil
}

func (g *descriptorGraph) addSemanticFiles(
	descriptor protoreflect.Descriptor,
) error {
	var references []protoreflect.Descriptor
	switch value := descriptor.(type) {
	case protoreflect.FieldDescriptor:
		if message := value.ContainingMessage(); message != nil {
			references = append(references, message)
		}
		if message := value.Message(); message != nil {
			references = append(references, message)
		}
		if enum := value.Enum(); enum != nil {
			references = append(references, enum)
		}
		if enumValue := value.DefaultEnumValue(); enumValue != nil {
			references = append(references, enumValue)
		}
	case protoreflect.MethodDescriptor:
		if input := value.Input(); input != nil {
			references = append(references, input)
		}
		if output := value.Output(); output != nil {
			references = append(references, output)
		}
	}
	for _, reference := range references {
		if err := g.addFile(reference.ParentFile()); err != nil {
			return fmt.Errorf(
				"symbol %q reference: %w",
				descriptor.FullName(),
				err,
			)
		}
	}
	return nil
}

func (g *descriptorGraph) compatible(other *descriptorGraph) error {
	for path, descriptor := range other.files {
		if existing, ok := g.files[path]; ok && existing != descriptor {
			return fmt.Errorf("file %q conflicts with the runtime descriptor graph", path)
		}
	}
	for name := range other.packages {
		if _, ok := g.symbols[name]; ok {
			return fmt.Errorf("package %q conflicts with a runtime symbol", name)
		}
	}
	for name, descriptor := range other.symbols {
		if _, ok := g.packages[name]; ok {
			return fmt.Errorf("symbol %q conflicts with a runtime package", name)
		}
		if existing, ok := g.symbols[name]; ok && existing != descriptor {
			return fmt.Errorf("symbol %q conflicts with the runtime descriptor graph", name)
		}
	}
	for key, descriptor := range other.extensions {
		if existing, ok := g.extensions[key]; ok && existing != descriptor {
			return fmt.Errorf(
				"extension %q conflicts with runtime extension %q at %s field number %d",
				descriptor.FullName(),
				existing.FullName(),
				key.message,
				key.number,
			)
		}
	}
	return nil
}

func descriptorFileGraph(file protoreflect.FileDescriptor) (*descriptorGraph, error) {
	graph := newDescriptorGraph()
	if err := graph.addFile(file); err != nil {
		return nil, err
	}
	return graph, nil
}

func walkDescriptors(
	file protoreflect.FileDescriptor,
	visit func(protoreflect.Descriptor) bool,
) {
	walkEnum := func(enum protoreflect.EnumDescriptor) bool {
		if !visit(enum) {
			return false
		}
		for index := 0; index < enum.Values().Len(); index++ {
			if !visit(enum.Values().Get(index)) {
				return false
			}
		}
		return true
	}
	var walkMessage func(protoreflect.MessageDescriptor) bool
	walkMessage = func(message protoreflect.MessageDescriptor) bool {
		if !visit(message) {
			return false
		}
		for index := 0; index < message.Fields().Len(); index++ {
			if !visit(message.Fields().Get(index)) {
				return false
			}
		}
		for index := 0; index < message.Oneofs().Len(); index++ {
			if !visit(message.Oneofs().Get(index)) {
				return false
			}
		}
		for index := 0; index < message.Enums().Len(); index++ {
			if !walkEnum(message.Enums().Get(index)) {
				return false
			}
		}
		for index := 0; index < message.Extensions().Len(); index++ {
			if !visit(message.Extensions().Get(index)) {
				return false
			}
		}
		for index := 0; index < message.Messages().Len(); index++ {
			if !walkMessage(message.Messages().Get(index)) {
				return false
			}
		}
		return true
	}
	for index := 0; index < file.Messages().Len(); index++ {
		if !walkMessage(file.Messages().Get(index)) {
			return
		}
	}
	for index := 0; index < file.Enums().Len(); index++ {
		if !walkEnum(file.Enums().Get(index)) {
			return
		}
	}
	for index := 0; index < file.Extensions().Len(); index++ {
		if !visit(file.Extensions().Get(index)) {
			return
		}
	}
	for index := 0; index < file.Services().Len(); index++ {
		service := file.Services().Get(index)
		if !visit(service) {
			return
		}
		for methodIndex := 0; methodIndex < service.Methods().Len(); methodIndex++ {
			if !visit(service.Methods().Get(methodIndex)) {
				return
			}
		}
	}
}
