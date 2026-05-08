package gojaprotobuf

import (
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestRuntimeStateSnapshotsBaseRegistries(t *testing.T) {
	host, sourceFiles := extensionHost(t, "snapshot_host.proto", "snapshot")
	early := extensionDescriptor(
		t,
		sourceFiles,
		host,
		"snapshot_early.proto",
		"early",
		100,
	)
	if err := sourceFiles.RegisterFile(early.ParentFile()); err != nil {
		t.Fatal(err)
	}
	sourceTypes := new(protoregistry.Types)
	if err := sourceTypes.RegisterMessage(dynamicpb.NewMessageType(host)); err != nil {
		t.Fatal(err)
	}
	if err := sourceTypes.RegisterExtension(dynamicpb.NewExtensionType(early)); err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	module, err := New(
		runtime,
		WithResolver(sourceTypes),
		WithFiles(sourceFiles),
	)
	if err != nil {
		t.Fatal(err)
	}
	earlyType, err := module.state.baseTypes.FindExtensionByName(early.FullName())
	if err != nil || earlyType.TypeDescriptor().Descriptor() != early {
		t.Fatalf("early extension identity = %v, %v", earlyType, err)
	}

	late := extensionDescriptor(
		t,
		sourceFiles,
		host,
		"snapshot_late.proto",
		"late",
		101,
	)
	if err := sourceFiles.RegisterFile(late.ParentFile()); err != nil {
		t.Fatal(err)
	}
	if err := sourceTypes.RegisterExtension(dynamicpb.NewExtensionType(late)); err != nil {
		t.Fatal(err)
	}
	if _, err := module.TypeResolver().FindExtensionByName(late.FullName()); err == nil {
		t.Fatal("late source type mutation entered the runtime snapshot")
	}
	if _, err := module.FileResolver().FindFileByPath(late.ParentFile().Path()); err == nil {
		t.Fatal("late source file mutation entered the runtime snapshot")
	}
	var numbers []protoreflect.FieldNumber
	module.TypeResolver().RangeExtensionsByMessage(
		host.FullName(),
		func(extension protoreflect.ExtensionType) bool {
			numbers = append(numbers, extension.TypeDescriptor().Number())
			return true
		},
	)
	if len(numbers) != 1 || numbers[0] != early.Number() {
		t.Fatalf("extension snapshot = %v, want [%d]", numbers, early.Number())
	}
	second, err := New(
		runtime,
		WithResolver(sourceTypes),
		WithFiles(sourceFiles),
	)
	if err != nil {
		t.Fatal(err)
	}
	if second.state != module.state {
		t.Fatal("same source identities did not reuse the runtime snapshot")
	}
}

func TestBaseDescriptorGraphRejectsConflicts(t *testing.T) {
	tests := []struct {
		name  string
		build func(*testing.T) (*protoregistry.Types, *protoregistry.Files)
	}{
		{
			name: "package and symbol",
			build: func(t *testing.T) (*protoregistry.Types, *protoregistry.Files) {
				typeFile := mustDescriptorFile(
					t,
					descriptorFile("package_type.proto", "graphpkg", "Outer"),
					nil,
				)
				packageFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("package_file.proto"),
					Package: new("graphpkg.Outer"),
					Syntax:  new("proto3"),
				}, nil)
				types := new(protoregistry.Types)
				if err := types.RegisterMessage(
					dynamicpb.NewMessageType(typeFile.Messages().Get(0)),
				); err != nil {
					t.Fatal(err)
				}
				files := new(protoregistry.Files)
				if err := files.RegisterFile(packageFile); err != nil {
					t.Fatal(err)
				}
				return types, files
			},
		},
		{
			name: "divergent file path",
			build: func(t *testing.T) (*protoregistry.Types, *protoregistry.Files) {
				typeFile := mustDescriptorFile(
					t,
					descriptorFile("shared_path.proto", "typepath", "TypeMessage"),
					nil,
				)
				registryFile := mustDescriptorFile(
					t,
					descriptorFile("shared_path.proto", "filepath", "FileMessage"),
					nil,
				)
				types := new(protoregistry.Types)
				if err := types.RegisterMessage(
					dynamicpb.NewMessageType(typeFile.Messages().Get(0)),
				); err != nil {
					t.Fatal(err)
				}
				files := new(protoregistry.Files)
				if err := files.RegisterFile(registryFile); err != nil {
					t.Fatal(err)
				}
				return types, files
			},
		},
		{
			name: "reachable enum value and type",
			build: func(t *testing.T) (*protoregistry.Types, *protoregistry.Files) {
				enumFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("enum_root.proto"),
					Package: new("valueroot"),
					Syntax:  new("proto3"),
					EnumType: []*descriptorpb.EnumDescriptorProto{{
						Name: new("Choice"),
						Value: []*descriptorpb.EnumValueDescriptorProto{{
							Name:   new("SHARED"),
							Number: proto.Int32(0),
						}},
					}},
				}, nil)
				messageFile := mustDescriptorFile(
					t,
					descriptorFile("message_root.proto", "valueroot", "SHARED"),
					nil,
				)
				types := new(protoregistry.Types)
				if err := types.RegisterEnum(
					dynamicpb.NewEnumType(enumFile.Enums().Get(0)),
				); err != nil {
					t.Fatal(err)
				}
				if err := types.RegisterMessage(
					dynamicpb.NewMessageType(messageFile.Messages().Get(0)),
				); err != nil {
					t.Fatal(err)
				}
				return types, new(protoregistry.Files)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			types, files := test.build(t)
			if _, err := New(
				goja.New(),
				WithResolver(types),
				WithFiles(files),
			); err == nil {
				t.Fatal("conflicting base graph was accepted")
			}
		})
	}
}

func TestBaseDescriptorGraphRejectsFileExtensionNumberCollision(t *testing.T) {
	host, files := extensionHost(t, "file_number_host.proto", "filenumber")
	first := extensionDescriptor(
		t,
		files,
		host,
		"file_number_first.proto",
		"first",
		100,
	)
	second := extensionDescriptor(
		t,
		files,
		host,
		"file_number_second.proto",
		"second",
		100,
	)
	if err := files.RegisterFile(first.ParentFile()); err != nil {
		t.Fatal(err)
	}
	if err := files.RegisterFile(second.ParentFile()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(
		goja.New(),
		WithResolver(new(protoregistry.Types)),
		WithFiles(files),
	); err == nil {
		t.Fatal("files-only extension number collision was accepted")
	}
}

func TestBaseDescriptorGraphAcceptsExactIdentity(t *testing.T) {
	file := mustDescriptorFile(
		t,
		descriptorFile("exact_identity.proto", "exactidentity", "Message"),
		nil,
	)
	types := new(protoregistry.Types)
	messageType := dynamicpb.NewMessageType(file.Messages().Get(0))
	if err := types.RegisterMessage(messageType); err != nil {
		t.Fatal(err)
	}
	files := new(protoregistry.Files)
	if err := files.RegisterFile(file); err != nil {
		t.Fatal(err)
	}
	module, err := New(
		goja.New(),
		WithResolver(types),
		WithFiles(files),
	)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := module.TypeResolver().FindMessageByName(
		messageType.Descriptor().FullName(),
	)
	if err != nil || resolved != messageType {
		t.Fatalf("resolved type identity = %v, %v", resolved, err)
	}
}

func TestDescriptorInstallChecksCompleteBaseGraph(t *testing.T) {
	t.Run("types-only file path", func(t *testing.T) {
		base := mustDescriptorFile(
			t,
			descriptorFile("dynamic_shared.proto", "basepath", "Base"),
			nil,
		)
		types := new(protoregistry.Types)
		if err := types.RegisterMessage(
			dynamicpb.NewMessageType(base.Messages().Get(0)),
		); err != nil {
			t.Fatal(err)
		}
		module, err := New(
			goja.New(),
			WithResolver(types),
			WithFiles(new(protoregistry.Files)),
		)
		if err != nil {
			t.Fatal(err)
		}
		data, err := proto.Marshal(
			descriptorFile("dynamic_shared.proto", "otherpath", "Other"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.loadFileDescriptorProtoBytes(data); err == nil {
			t.Fatal("dynamic file reused a types-only base path")
		}
		if _, err := module.FindDescriptor("otherpath.Other"); err == nil {
			t.Fatal("failed transaction published a descriptor")
		}
	})

	t.Run("base package", func(t *testing.T) {
		base := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
			Name:    new("base_package.proto"),
			Package: new("dynamicpkg.Outer"),
			Syntax:  new("proto3"),
		}, nil)
		files := new(protoregistry.Files)
		if err := files.RegisterFile(base); err != nil {
			t.Fatal(err)
		}
		module, err := New(
			goja.New(),
			WithResolver(new(protoregistry.Types)),
			WithFiles(files),
		)
		if err != nil {
			t.Fatal(err)
		}
		data, err := proto.Marshal(
			descriptorFile("dynamic_symbol.proto", "dynamicpkg", "Outer"),
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.loadFileDescriptorProtoBytes(data); err == nil {
			t.Fatal("dynamic symbol shadowed a base package")
		}
		if _, err := module.FindDescriptor("dynamicpkg.Outer"); err == nil {
			t.Fatal("failed transaction published a descriptor")
		}
	})
}

func TestWellKnownHelpersUseReachableBaseGraph(t *testing.T) {
	root, expected := reachableWellKnownRoot(t)
	tests := []struct {
		name  string
		types func(*testing.T) *protoregistry.Types
		files func(*testing.T) *protoregistry.Files
	}{
		{
			name: "types root",
			types: func(t *testing.T) *protoregistry.Types {
				t.Helper()
				types := new(protoregistry.Types)
				if err := types.RegisterMessage(
					dynamicpb.NewMessageType(root.Messages().Get(0)),
				); err != nil {
					t.Fatal(err)
				}
				return types
			},
			files: func(*testing.T) *protoregistry.Files {
				return new(protoregistry.Files)
			},
		},
		{
			name: "files root",
			types: func(*testing.T) *protoregistry.Types {
				return new(protoregistry.Types)
			},
			files: func(t *testing.T) *protoregistry.Files {
				t.Helper()
				files := new(protoregistry.Files)
				if err := files.RegisterFile(root); err != nil {
					t.Fatal(err)
				}
				return files
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exerciseWellKnownGraph(
				t,
				test.types(t),
				test.files(t),
				expected,
			)
		})
	}
}

func TestFilesOnlyGraphMaterializesCanonicalTypes(t *testing.T) {
	file := filesOnlyTypeGraph(t)
	sourceTypes := new(protoregistry.Types)
	files := new(protoregistry.Files)
	if err := files.RegisterFile(file); err != nil {
		t.Fatal(err)
	}
	module, err := New(
		goja.New(),
		WithResolver(sourceTypes),
		WithFiles(files),
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := module.TypeResolver()
	for _, name := range []protoreflect.FullName{
		"fileonly.Payload",
		"fileonly.Envelope",
		"fileonly.Host",
		"google.protobuf.Any",
		"google.protobuf.Timestamp",
	} {
		descriptor, err := module.FindDescriptor(name)
		if err != nil {
			t.Fatalf("descriptor %q: %v", name, err)
		}
		message, err := resolver.FindMessageByName(name)
		if err != nil {
			t.Fatalf("message type %q: %v", name, err)
		}
		if message.Descriptor() != descriptor {
			t.Fatalf(
				"message type %q descriptor = %p, want %p",
				name,
				message.Descriptor(),
				descriptor,
			)
		}
		again, err := resolver.FindMessageByURL(
			"type.example.test/" + string(name),
		)
		if err != nil {
			t.Fatalf("message URL %q: %v", name, err)
		}
		if again != message {
			t.Fatalf("message type %q identity was not stable", name)
		}
	}
	resolvedAny, err := resolver.FindMessageByName("google.protobuf.Any")
	if err != nil {
		t.Fatal(err)
	}
	if module.state.anyType != resolvedAny {
		t.Fatal("Any helper and resolver use different type identities")
	}
	resolvedTimestamp, err := resolver.FindMessageByName(
		"google.protobuf.Timestamp",
	)
	if err != nil {
		t.Fatal(err)
	}
	if module.state.timestampType != resolvedTimestamp {
		t.Fatal("Timestamp helper and resolver use different type identities")
	}

	enumDescriptor := file.Enums().ByName("Mode")
	enumType, err := module.state.baseTypes.FindEnumByName(
		enumDescriptor.FullName(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if enumType.Descriptor() != enumDescriptor {
		t.Fatal("files-only enum type has a divergent descriptor")
	}

	extensionDescriptor := file.Extensions().ByName("extra")
	extensionByName, err := resolver.FindExtensionByName(
		extensionDescriptor.FullName(),
	)
	if err != nil {
		t.Fatal(err)
	}
	extensionByNumber, err := resolver.FindExtensionByNumber(
		extensionDescriptor.ContainingMessage().FullName(),
		extensionDescriptor.Number(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if extensionByName != extensionByNumber ||
		extensionByName.TypeDescriptor().Descriptor() != extensionDescriptor {
		t.Fatal("files-only extension type identity was not stable")
	}
	var ranged []protoreflect.ExtensionType
	resolver.RangeExtensionsByMessage(
		extensionDescriptor.ContainingMessage().FullName(),
		func(extension protoreflect.ExtensionType) bool {
			ranged = append(ranged, extension)
			return true
		},
	)
	if len(ranged) != 1 || ranged[0] != extensionByName {
		t.Fatalf("ranged extensions = %v, want exact configured extension", ranged)
	}
	for _, dependency := range []protoreflect.FileDescriptor{
		anypb.File_google_protobuf_any_proto,
		timestamppb.File_google_protobuf_timestamp_proto,
	} {
		resolved, err := module.FileResolver().FindFileByPath(dependency.Path())
		if err != nil {
			t.Fatalf("reachable file %q: %v", dependency.Path(), err)
		}
		if resolved != dependency {
			t.Fatalf("reachable file %q has divergent identity", dependency.Path())
		}
	}
	for _, descriptor := range []protoreflect.Descriptor{
		enumDescriptor,
		extensionDescriptor,
	} {
		resolved, err := module.FindDescriptor(descriptor.FullName())
		if err != nil {
			t.Fatalf("reachable descriptor %q: %v", descriptor.FullName(), err)
		}
		if resolved != descriptor {
			t.Fatalf("reachable descriptor %q has divergent identity", descriptor.FullName())
		}
	}
	var sourceTypeCount int
	sourceTypes.RangeMessages(func(protoreflect.MessageType) bool {
		sourceTypeCount++
		return true
	})
	sourceTypes.RangeEnums(func(protoreflect.EnumType) bool {
		sourceTypeCount++
		return true
	})
	sourceTypes.RangeExtensions(func(protoreflect.ExtensionType) bool {
		sourceTypeCount++
		return true
	})
	if sourceTypeCount != 0 {
		t.Fatalf("materialization mutated source types with %d entries", sourceTypeCount)
	}
	var sourceFileCount int
	files.RangeFiles(func(source protoreflect.FileDescriptor) bool {
		sourceFileCount++
		if source != file {
			t.Fatalf("source files contain unexpected file %q", source.Path())
		}
		return true
	})
	if sourceFileCount != 1 {
		t.Fatalf("source file count = %d, want 1", sourceFileCount)
	}

	second, err := New(
		module.runtime,
		WithResolver(sourceTypes),
		WithFiles(files),
	)
	if err != nil {
		t.Fatal(err)
	}
	secondPayload, err := second.TypeResolver().FindMessageByName(
		"fileonly.Payload",
	)
	if err != nil {
		t.Fatal(err)
	}
	firstPayload, err := resolver.FindMessageByName("fileonly.Payload")
	if err != nil {
		t.Fatal(err)
	}
	if secondPayload != firstPayload {
		t.Fatal("second module produced a duplicate files-only type")
	}
}

func TestGraphMaterializationRetainsRegisteredType(t *testing.T) {
	file := filesOnlyTypeGraph(t)
	files := new(protoregistry.Files)
	if err := files.RegisterFile(file); err != nil {
		t.Fatal(err)
	}
	types := new(protoregistry.Types)
	payload := dynamicpb.NewMessageType(file.Messages().ByName("Payload"))
	if err := types.RegisterMessage(payload); err != nil {
		t.Fatal(err)
	}
	module, err := New(
		goja.New(),
		WithResolver(types),
		WithFiles(files),
	)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := module.TypeResolver().FindMessageByName(
		"fileonly.Payload",
	)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != payload {
		t.Fatal("materialization replaced an explicitly registered type")
	}
	if _, err := module.TypeResolver().FindMessageByName(
		"fileonly.Envelope",
	); err != nil {
		t.Fatalf("files-only sibling type was not materialized: %v", err)
	}
}

func filesOnlyTypeGraph(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	dependencies := new(protoregistry.Files)
	for _, dependency := range []protoreflect.FileDescriptor{
		anypb.File_google_protobuf_any_proto,
		timestamppb.File_google_protobuf_timestamp_proto,
	} {
		if err := dependencies.RegisterFile(dependency); err != nil {
			t.Fatal(err)
		}
	}
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	message := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum()
	return mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:       new("fileonly/root.proto"),
		Package:    new("fileonly"),
		Syntax:     new("proto2"),
		Dependency: []string{anypb.File_google_protobuf_any_proto.Path(), timestamppb.File_google_protobuf_timestamp_proto.Path()},
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: new("Mode"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: new("MODE_UNSPECIFIED"), Number: proto.Int32(0)},
				{Name: new("MODE_ACTIVE"), Number: proto.Int32(1)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("Payload"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("value"),
						JsonName: new("value"),
						Number:   proto.Int32(1),
						Label:    optional,
						Type:     int64Type,
					},
					{
						Name:     new("mode"),
						JsonName: new("mode"),
						Number:   proto.Int32(2),
						Label:    optional,
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: new(".fileonly.Mode"),
					},
				},
			},
			{
				Name: new("Envelope"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("payload"),
						JsonName: new("payload"),
						Number:   proto.Int32(1),
						Label:    optional,
						Type:     message,
						TypeName: new(".google.protobuf.Any"),
					},
					{
						Name:     new("created_at"),
						JsonName: new("createdAt"),
						Number:   proto.Int32(2),
						Label:    optional,
						Type:     message,
						TypeName: new(".google.protobuf.Timestamp"),
					},
				},
			},
			{
				Name: new("Host"),
				ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
					Start: proto.Int32(100),
					End:   proto.Int32(200),
				}},
			},
		},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("extra"),
			JsonName: new("extra"),
			Number:   proto.Int32(100),
			Label:    optional,
			Type:     int64Type,
			Extendee: new(".fileonly.Host"),
		}},
	}, dependencies)
}

func reachableWellKnownRoot(
	t *testing.T,
) (protoreflect.FileDescriptor, map[protoreflect.FullName]protoreflect.MessageDescriptor) {
	t.Helper()
	dependencies := new(protoregistry.Files)
	expected := make(map[protoreflect.FullName]protoreflect.MessageDescriptor)
	for _, generated := range []protoreflect.FileDescriptor{
		timestamppb.File_google_protobuf_timestamp_proto,
		durationpb.File_google_protobuf_duration_proto,
		anypb.File_google_protobuf_any_proto,
	} {
		dynamic, err := protodesc.NewFile(
			protodesc.ToFileDescriptorProto(generated),
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := dependencies.RegisterFile(dynamic); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < dynamic.Messages().Len(); index++ {
			message := dynamic.Messages().Get(index)
			expected[message.FullName()] = message
		}
	}
	root, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    new("reachable_well_known.proto"),
		Package: new("reachablewkt"),
		Syntax:  new("proto3"),
		Dependency: []string{
			timestampDesc.ParentFile().Path(),
			durationDesc.ParentFile().Path(),
			anyDesc.ParentFile().Path(),
		},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Root"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     new("timestamp"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: new(".google.protobuf.Timestamp"),
				},
				{
					Name:     new("duration"),
					Number:   proto.Int32(2),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: new(".google.protobuf.Duration"),
				},
				{
					Name:     new("payload"),
					Number:   proto.Int32(3),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: new(".google.protobuf.Any"),
				},
			},
		}},
	}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	return root, expected
}

func mustDescriptorFile(
	t *testing.T,
	file *descriptorpb.FileDescriptorProto,
	resolver protodesc.Resolver,
) protoreflect.FileDescriptor {
	t.Helper()
	descriptor, err := protodesc.NewFile(file, resolver)
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}
