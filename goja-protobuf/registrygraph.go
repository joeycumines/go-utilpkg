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

// covers reports whether base semantically covers other: every file path,
// package, symbol, and extension that other declares must already be present
// in base with a matching descriptor kind. Unlike [descriptorGraph.compatible],
// it is intended for descriptors built from different registries (so pointer
// identity does not hold) and compares by fully-qualified name and kind only.
//
// It is the gate that lets an incoming descriptor file whose bytes differ from
// the base only by additive, non-semantic metadata (for example the custom
// FileOptions extension ranges that buf injects into
// google/protobuf/descriptor.proto) be satisfied from the base registry without
// being re-registered, while still rejecting a genuine redefinition that
// introduces a symbol or extension the base does not have.
//
// Because coverage is verified against base, an incoming file that passes is
// discarded: the [combinedFileResolver] resolves its path and symbols from base
// (local miss falls through to global, then the base graph), so registering a
// local copy would only shadow the canonical base identity.
func (g *descriptorGraph) covers(other *descriptorGraph) error {
	// Iterate every category in a deterministic, sorted order. A coverage gate
	// answers the same question for the same input, so the FIRST conflict it
	// surfaces — and therefore the error string a caller sees — must be stable
	// across runs. Ranging over Go maps directly yields a randomized order, which
	// makes the reported conflict (and thus logs and error-dependent tests)
	// non-reproducible when more than one violation is present. Sorting by the
	// natural ordering of each key guarantees a single, reproducible verdict.
	files := make([]string, 0, len(other.files))
	for path := range other.files {
		files = append(files, path)
	}
	slices.Sort(files)
	for _, path := range files {
		descriptor := other.files[path]
		base, ok := g.files[path]
		if !ok {
			return fmt.Errorf("file %q is not present in the base registry", path)
		}
		if base.Syntax() != descriptor.Syntax() {
			return fmt.Errorf(
				"file %q syntax changed from %v to %v",
				path,
				base.Syntax(),
				descriptor.Syntax(),
			)
		}
		if base.FullName() != descriptor.FullName() {
			return fmt.Errorf(
				"file %q package changed from %q to %q",
				path,
				base.FullName(),
				descriptor.FullName(),
			)
		}
	}
	packages := make([]protoreflect.FullName, 0, len(other.packages))
	for name := range other.packages {
		packages = append(packages, name)
	}
	slices.Sort(packages)
	for _, name := range packages {
		if _, ok := g.packages[name]; !ok {
			return fmt.Errorf("package %q is not present in the base registry", name)
		}
	}
	symbols := make([]protoreflect.FullName, 0, len(other.symbols))
	for name := range other.symbols {
		symbols = append(symbols, name)
	}
	slices.Sort(symbols)
	for _, name := range symbols {
		descriptor := other.symbols[name]
		base, ok := g.symbols[name]
		if !ok {
			return fmt.Errorf(
				"symbol %q declared in %q is not present in the base registry",
				name,
				descriptor.ParentFile().Path(),
			)
		}
		if baseKind := symbolKind(base); baseKind != symbolKind(descriptor) {
			return fmt.Errorf(
				"symbol %q changed kind from %s to %s",
				name,
				baseKind,
				symbolKind(descriptor),
			)
		}
		if err := compareDescriptors(base, descriptor); err != nil {
			return err
		}
	}
	extensions := make([]extensionNumberKey, 0, len(other.extensions))
	for key := range other.extensions {
		extensions = append(extensions, key)
	}
	slices.SortFunc(extensions, func(a, b extensionNumberKey) int {
		if a.message != b.message {
			if a.message < b.message {
				return -1
			}
			return 1
		}
		switch {
		case a.number < b.number:
			return -1
		case a.number > b.number:
			return 1
		default:
			return 0
		}
	})
	for _, key := range extensions {
		descriptor := other.extensions[key]
		base, ok := g.extensions[key]
		if !ok {
			return fmt.Errorf(
				"extension %q (%s field %d) is not present in the base registry",
				descriptor.FullName(),
				key.message,
				key.number,
			)
		}
		if base.FullName() != descriptor.FullName() {
			return fmt.Errorf(
				"extension at %s field %d changed from %q to %q",
				key.message,
				key.number,
				base.FullName(),
				descriptor.FullName(),
			)
		}
		if err := compareDescriptors(base, descriptor); err != nil {
			return err
		}
	}
	return nil
}

func compareDescriptors(base, descriptor protoreflect.Descriptor) error {
	if base.ParentFile().Path() != descriptor.ParentFile().Path() {
		return fmt.Errorf(
			"symbol %q defining file changed from %q to %q",
			descriptor.FullName(),
			base.ParentFile().Path(),
			descriptor.ParentFile().Path(),
		)
	}
	if baseParent, descParent := base.Parent(), descriptor.Parent(); baseParent != nil && descParent != nil {
		if baseParent.FullName() != descParent.FullName() {
			return fmt.Errorf(
				"symbol %q parent changed from %q to %q",
				descriptor.FullName(),
				baseParent.FullName(),
				descParent.FullName(),
			)
		}
	}

	switch desc := descriptor.(type) {
	case protoreflect.MessageDescriptor:
		baseMsg, ok := base.(protoreflect.MessageDescriptor)
		if !ok {
			return fmt.Errorf("symbol %q changed kind from %s to message", descriptor.FullName(), symbolKind(base))
		}
		if baseMsg.IsMapEntry() != desc.IsMapEntry() {
			return fmt.Errorf("message %q map-entry semantics mismatch", descriptor.FullName())
		}
		for i := 0; i < desc.Fields().Len(); i++ {
			field := desc.Fields().Get(i)
			baseField := baseMsg.Fields().ByNumber(field.Number())
			if baseField == nil {
				return fmt.Errorf(
					"field %q (%d) in message %q is not present in base registry",
					field.Name(),
					field.Number(),
					descriptor.FullName(),
				)
			}
			if baseField.Name() != field.Name() {
				return fmt.Errorf(
					"field number %d in message %q name changed from %q to %q",
					field.Number(),
					descriptor.FullName(),
					baseField.Name(),
					field.Name(),
				)
			}
			if baseField.Kind() != field.Kind() {
				return fmt.Errorf(
					"field %q in message %q type changed from %v to %v",
					field.Name(),
					descriptor.FullName(),
					baseField.Kind(),
					field.Kind(),
				)
			}
			if baseField.Cardinality() != field.Cardinality() {
				return fmt.Errorf(
					"field %q in message %q cardinality changed from %v to %v",
					field.Name(),
					descriptor.FullName(),
					baseField.Cardinality(),
					field.Cardinality(),
				)
			}
			if field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind {
				if baseField.Message() == nil || field.Message() == nil ||
					baseField.Message().FullName() != field.Message().FullName() {
					return fmt.Errorf(
						"field %q in message %q message type mismatch",
						field.Name(),
						descriptor.FullName(),
					)
				}
			}
			if field.Kind() == protoreflect.EnumKind {
				if baseField.Enum() == nil || field.Enum() == nil ||
					baseField.Enum().FullName() != field.Enum().FullName() {
					return fmt.Errorf(
						"field %q in message %q enum type mismatch",
						field.Name(),
						descriptor.FullName(),
					)
				}
			}
			if (baseField.ContainingOneof() == nil) != (field.ContainingOneof() == nil) {
				return fmt.Errorf(
					"field %q in message %q oneof presence mismatch",
					field.Name(),
					descriptor.FullName(),
				)
			}
			if baseOneof, descOneof := baseField.ContainingOneof(), field.ContainingOneof(); baseOneof != nil && descOneof != nil {
				if !baseOneof.IsSynthetic() && !descOneof.IsSynthetic() && baseOneof.Name() != descOneof.Name() {
					return fmt.Errorf(
						"field %q in message %q oneof changed from %q to %q",
						field.Name(),
						descriptor.FullName(),
						baseOneof.Name(),
						descOneof.Name(),
					)
				}
			}
			if baseField.IsPacked() != field.IsPacked() {
				return fmt.Errorf(
					"field %q in message %q packed attribute mismatch",
					field.Name(),
					descriptor.FullName(),
				)
			}
			if baseField.HasPresence() != field.HasPresence() {
				return fmt.Errorf(
					"field %q in message %q presence semantics mismatch",
					field.Name(),
					descriptor.FullName(),
				)
			}
		}

	case protoreflect.EnumDescriptor:
		baseEnum, ok := base.(protoreflect.EnumDescriptor)
		if !ok {
			return fmt.Errorf("symbol %q changed kind from %s to enum", descriptor.FullName(), symbolKind(base))
		}
		for i := 0; i < desc.Values().Len(); i++ {
			val := desc.Values().Get(i)
			baseVal := baseEnum.Values().ByName(val.Name())
			if baseVal == nil {
				return fmt.Errorf(
					"enum value %q in enum %q is not present in base registry",
					val.Name(),
					descriptor.FullName(),
				)
			}
			if baseVal.Number() != val.Number() {
				return fmt.Errorf(
					"enum value %q in enum %q number changed from %d to %d",
					val.Name(),
					descriptor.FullName(),
					baseVal.Number(),
					val.Number(),
				)
			}
		}

	case protoreflect.EnumValueDescriptor:
		baseVal, ok := base.(protoreflect.EnumValueDescriptor)
		if !ok {
			return fmt.Errorf("symbol %q changed kind from %s to enumvalue", descriptor.FullName(), symbolKind(base))
		}
		if baseVal.Number() != desc.Number() {
			return fmt.Errorf(
				"enum value %q number changed from %d to %d",
				descriptor.FullName(),
				baseVal.Number(),
				desc.Number(),
			)
		}

	case protoreflect.FieldDescriptor:
		baseField, ok := base.(protoreflect.FieldDescriptor)
		if !ok {
			return fmt.Errorf("symbol %q changed kind from %s to field", descriptor.FullName(), symbolKind(base))
		}
		if baseField.IsExtension() != desc.IsExtension() {
			return fmt.Errorf("symbol %q extension kind mismatch", descriptor.FullName())
		}
		if baseField.Number() != desc.Number() {
			return fmt.Errorf(
				"field %q number changed from %d to %d",
				descriptor.FullName(),
				baseField.Number(),
				desc.Number(),
			)
		}
		if baseField.Kind() != desc.Kind() {
			return fmt.Errorf(
				"field %q type changed from %v to %v",
				descriptor.FullName(),
				baseField.Kind(),
				desc.Kind(),
			)
		}
		if baseField.Cardinality() != desc.Cardinality() {
			return fmt.Errorf(
				"field %q cardinality changed from %v to %v",
				descriptor.FullName(),
				baseField.Cardinality(),
				desc.Cardinality(),
			)
		}
		// A FieldDescriptor is reached directly either as a top-level extension
		// or as a standalone field symbol. The message-field loop already
		// verifies target-type, packing, and presence semantics, so mirror them
		// here to keep the field case self-sufficient and to close the coverage
		// hole that let an otherwise-covered extension silently retarget its
		// message or enum type, flip its packing, or change its presence.
		if desc.Kind() == protoreflect.MessageKind || desc.Kind() == protoreflect.GroupKind {
			if baseField.Message() == nil || desc.Message() == nil ||
				baseField.Message().FullName() != desc.Message().FullName() {
				return fmt.Errorf(
					"field %q message type mismatch",
					descriptor.FullName(),
				)
			}
		}
		if desc.Kind() == protoreflect.EnumKind {
			if baseField.Enum() == nil || desc.Enum() == nil ||
				baseField.Enum().FullName() != desc.Enum().FullName() {
				return fmt.Errorf(
					"field %q enum type mismatch",
					descriptor.FullName(),
				)
			}
		}
		if baseField.IsPacked() != desc.IsPacked() {
			return fmt.Errorf(
				"field %q packed attribute mismatch",
				descriptor.FullName(),
			)
		}
		if baseField.HasPresence() != desc.HasPresence() {
			return fmt.Errorf(
				"field %q presence semantics mismatch",
				descriptor.FullName(),
			)
		}
		if desc.IsExtension() {
			baseContaining := baseField.ContainingMessage()
			containing := desc.ContainingMessage()
			if baseContaining == nil || containing == nil ||
				baseContaining.FullName() != containing.FullName() {
				return fmt.Errorf(
					"extension %q extendee changed from %s to %s",
					descriptor.FullName(),
					extendeeName(baseContaining),
					extendeeName(containing),
				)
			}
		}

	case protoreflect.ServiceDescriptor:
		baseSvc, ok := base.(protoreflect.ServiceDescriptor)
		if !ok {
			return fmt.Errorf("symbol %q changed kind from %s to service", descriptor.FullName(), symbolKind(base))
		}
		for i := 0; i < desc.Methods().Len(); i++ {
			method := desc.Methods().Get(i)
			baseMethod := baseSvc.Methods().ByName(method.Name())
			if baseMethod == nil {
				return fmt.Errorf(
					"method %q in service %q is not present in base registry",
					method.Name(),
					descriptor.FullName(),
				)
			}
			if baseMethod.Input().FullName() != method.Input().FullName() {
				return fmt.Errorf(
					"method %q in service %q input type changed from %q to %q",
					method.Name(),
					descriptor.FullName(),
					baseMethod.Input().FullName(),
					method.Input().FullName(),
				)
			}
			if baseMethod.Output().FullName() != method.Output().FullName() {
				return fmt.Errorf(
					"method %q in service %q output type changed from %q to %q",
					method.Name(),
					descriptor.FullName(),
					baseMethod.Output().FullName(),
					method.Output().FullName(),
				)
			}
			if baseMethod.IsStreamingClient() != method.IsStreamingClient() {
				return fmt.Errorf(
					"method %q in service %q client streaming changed from %v to %v",
					method.Name(),
					descriptor.FullName(),
					baseMethod.IsStreamingClient(),
					method.IsStreamingClient(),
				)
			}
			if baseMethod.IsStreamingServer() != method.IsStreamingServer() {
				return fmt.Errorf(
					"method %q in service %q server streaming changed from %v to %v",
					method.Name(),
					descriptor.FullName(),
					baseMethod.IsStreamingServer(),
					method.IsStreamingServer(),
				)
			}
		}

	case protoreflect.MethodDescriptor:
		baseMethod, ok := base.(protoreflect.MethodDescriptor)
		if !ok {
			return fmt.Errorf("symbol %q changed kind from %s to method", descriptor.FullName(), symbolKind(base))
		}
		if baseMethod.Input().FullName() != desc.Input().FullName() {
			return fmt.Errorf(
				"method %q input type changed from %q to %q",
				descriptor.FullName(),
				baseMethod.Input().FullName(),
				desc.Input().FullName(),
			)
		}
		if baseMethod.Output().FullName() != desc.Output().FullName() {
			return fmt.Errorf(
				"method %q output type changed from %q to %q",
				descriptor.FullName(),
				baseMethod.Output().FullName(),
				desc.Output().FullName(),
			)
		}
		if baseMethod.IsStreamingClient() != desc.IsStreamingClient() {
			return fmt.Errorf(
				"method %q client streaming changed from %v to %v",
				descriptor.FullName(),
				baseMethod.IsStreamingClient(),
				desc.IsStreamingClient(),
			)
		}
		if baseMethod.IsStreamingServer() != desc.IsStreamingServer() {
			return fmt.Errorf(
				"method %q server streaming changed from %v to %v",
				descriptor.FullName(),
				baseMethod.IsStreamingServer(),
				desc.IsStreamingServer(),
			)
		}
	}
	return nil
}

// extendeeName renders a containing-message descriptor's full name for error
// reporting, using a stable placeholder when the descriptor is nil so an extendee
// mismatch is never formatted against a nil descriptor.
func extendeeName(descriptor protoreflect.MessageDescriptor) string {
	if descriptor == nil {
		return "<nil>"
	}
	return string(descriptor.FullName())
}

// symbolKind returns a coarse, stable classification for a descriptor used by
// [descriptorGraph.covers] to detect kind changes without relying on pointer
// identity. It mirrors the value/extension distinction that matters for
// resolution; finer distinctions (field vs oneof, message vs enum) are captured
// by the protoreflect type switches.
func symbolKind(descriptor protoreflect.Descriptor) string {
	switch d := descriptor.(type) {
	case protoreflect.MessageDescriptor:
		return "message"
	case protoreflect.EnumDescriptor:
		return "enum"
	case protoreflect.EnumValueDescriptor:
		return "enumvalue"
	case protoreflect.FieldDescriptor:
		if d.IsExtension() {
			return "extension"
		}
		return "field"
	case protoreflect.OneofDescriptor:
		return "oneof"
	case protoreflect.ServiceDescriptor:
		return "service"
	case protoreflect.MethodDescriptor:
		return "method"
	default:
		return fmt.Sprintf("%T", descriptor)
	}
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
