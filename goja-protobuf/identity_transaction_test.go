package gojaprotobuf

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"
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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRuntimeStateCanonical(t *testing.T) {
	runtime := goja.New()
	types := new(protoregistry.Types)
	files := new(protoregistry.Files)
	first, err := New(runtime, WithResolver(types), WithFiles(files))
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(runtime, WithResolver(types), WithFiles(files))
	if err != nil {
		t.Fatal(err)
	}
	if first.state != second.state {
		t.Fatal("modules for one runtime did not reuse canonical state")
	}
	if _, err := first.LoadDescriptorSetBytes(testDescriptorSetBytes()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.FindDescriptor("test.SimpleMessage"); err != nil {
		t.Fatalf("second module cannot see the shared descriptor graph: %v", err)
	}
	recovered := capturePanic(t, func() {
		_, _ = New(
			runtime,
			WithResolver(new(protoregistry.Types)),
			WithFiles(files),
		)
	})
	if got := fmt.Sprint(recovered); !strings.Contains(
		got,
		"different base registries",
	) {
		t.Fatalf("panic = %q", got)
	}
}

func TestNewReturnsDynamicRuntimeStateFailure(t *testing.T) {
	runtime := goja.New()
	if err := runtime.Set("WeakMap", goja.Undefined()); err != nil {
		t.Fatal(err)
	}
	module, err := New(runtime)
	if module != nil {
		t.Fatalf("module = %#v, want nil", module)
	}
	if err == nil || !strings.Contains(err.Error(), "WeakMap constructor") {
		t.Fatalf("error = %v, want dynamic WeakMap failure", err)
	}
}

func TestNewContainsAbruptWeakMapAccessAndCanRetry(t *testing.T) {
	tests := []struct {
		name   string
		broken string
		want   string
	}{
		{
			name: "constructor getter",
			broken: `
				Object.defineProperty(globalThis, "WeakMap", {
					configurable: true,
					get() { throw new Error("constructor getter failed"); }
				});
			`,
			want: "constructor getter failed",
		},
		{
			name: "instance get getter",
			broken: `
				globalThis.WeakMap = function () {
					return Object.defineProperty({}, "get", {
						get() { throw new Error("get getter failed"); }
					});
				};
			`,
			want: "get getter failed",
		},
		{
			name: "instance set getter",
			broken: `
				globalThis.WeakMap = function () {
					return Object.defineProperties({}, {
						get: { value() {} },
						set: {
							get() { throw new Error("set getter failed"); }
						}
					});
				};
			`,
			want: "set getter failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := goja.New()
			if _, err := runtime.RunString(
				`globalThis.__savedWeakMap = WeakMap;` + test.broken,
			); err != nil {
				t.Fatal(err)
			}
			module, err := New(runtime)
			if module != nil {
				t.Fatalf("module = %#v, want nil", module)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			state := runtime.GlobalObject().GetSymbol(runtimeStateSymbol)
			if state != nil && !goja.IsUndefined(state) {
				t.Fatal("failed construction installed runtime state")
			}
			if _, err := runtime.RunString(`
				Object.defineProperty(globalThis, "WeakMap", {
					configurable: true,
					writable: true,
					value: globalThis.__savedWeakMap
				});
				delete globalThis.__savedWeakMap;
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := New(runtime); err != nil {
				t.Fatalf("retry after restoring WeakMap: %v", err)
			}
		})
	}
}

func TestRuntimeStateRejectsDivergentBaseRegistryIdentity(t *testing.T) {
	fileProto := descriptorFile("divergent_base.proto", "divergentbase", "Message")
	typeFile, err := protodesc.NewFile(proto.Clone(fileProto).(*descriptorpb.FileDescriptorProto), nil)
	if err != nil {
		t.Fatal(err)
	}
	registryFile, err := protodesc.NewFile(proto.Clone(fileProto).(*descriptorpb.FileDescriptorProto), nil)
	if err != nil {
		t.Fatal(err)
	}
	types := new(protoregistry.Types)
	if err := types.RegisterMessage(dynamicpb.NewMessageType(typeFile.Messages().Get(0))); err != nil {
		t.Fatal(err)
	}
	files := new(protoregistry.Files)
	if err := files.RegisterFile(registryFile); err != nil {
		t.Fatal(err)
	}
	if _, err := New(goja.New(), WithResolver(types), WithFiles(files)); err == nil {
		t.Fatal("divergent base descriptor identities were accepted")
	}
}

func TestRuntimeStateRejectsBaseEnumValueFileSymbolCollision(t *testing.T) {
	typeFile, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    new("base_type_enum.proto"),
		Package: new("basecollision"),
		Syntax:  new("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: new("TypeEnum"),
			Value: []*descriptorpb.EnumValueDescriptorProto{{
				Name:   new("SHARED"),
				Number: proto.Int32(0),
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registryFile, err := protodesc.NewFile(
		descriptorFile("base_file_message.proto", "basecollision", "SHARED"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	types := new(protoregistry.Types)
	if err := types.RegisterEnum(dynamicpb.NewEnumType(typeFile.Enums().Get(0))); err != nil {
		t.Fatal(err)
	}
	files := new(protoregistry.Files)
	if err := files.RegisterFile(registryFile); err != nil {
		t.Fatal(err)
	}
	if _, err := New(goja.New(), WithResolver(types), WithFiles(files)); err == nil {
		t.Fatal("base enum-value and file symbol collision was accepted")
	}
}

func TestRuntimeStateRejectsNestedBaseDescriptorCollision(t *testing.T) {
	typeFile, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    new("nested_type.proto"),
		Package: new("nestedbase"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Outer"),
			NestedType: []*descriptorpb.DescriptorProto{{
				Name: new("Inner"),
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	registryFile, err := protodesc.NewFile(
		descriptorFile("nested_file.proto", "nestedbase.Outer", "Inner"),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	types := new(protoregistry.Types)
	if err := types.RegisterMessage(dynamicpb.NewMessageType(typeFile.Messages().Get(0))); err != nil {
		t.Fatal(err)
	}
	files := new(protoregistry.Files)
	if err := files.RegisterFile(registryFile); err != nil {
		t.Fatal(err)
	}
	if _, err := New(goja.New(), WithResolver(types), WithFiles(files)); err == nil {
		t.Fatal("nested base descriptor collision was accepted")
	}
}

func TestRuntimeStateRejectsSplitBaseExtensionNumberCollision(t *testing.T) {
	host, files := extensionHost(t, "split_extension_host.proto", "splitextension")
	typeExtension := extensionDescriptor(
		t,
		files,
		host,
		"split_type_extension.proto",
		"type_extension",
		100,
	)
	fileExtension := extensionDescriptor(
		t,
		files,
		host,
		"split_file_extension.proto",
		"file_extension",
		100,
	)
	if err := files.RegisterFile(fileExtension.ParentFile()); err != nil {
		t.Fatal(err)
	}
	types := new(protoregistry.Types)
	if err := types.RegisterExtension(dynamicpb.NewExtensionType(typeExtension)); err != nil {
		t.Fatal(err)
	}
	if _, err := New(goja.New(), WithResolver(types), WithFiles(files)); err == nil {
		t.Fatal("split base extension-number collision was accepted")
	}
}

func TestDescriptorGraphRejectsBaseFileExtensionNumberCollision(t *testing.T) {
	host, files := extensionHost(t, "local_extension_host.proto", "localextension")
	baseExtension := extensionDescriptor(
		t,
		files,
		host,
		"base_extension.proto",
		"base_extension",
		100,
	)
	if err := files.RegisterFile(baseExtension.ParentFile()); err != nil {
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
	local := extensionFileProto(
		"local_extension.proto",
		"localextension",
		"local_extension",
		100,
		host.ParentFile().Path(),
	)
	data, err := proto.Marshal(local)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.loadFileDescriptorProtoBytes(data); err == nil {
		t.Fatal("local extension reused a base file extension number")
	}
}

func TestWellKnownHelpersUseCanonicalRuntimeGraph(t *testing.T) {
	generatedDescriptors := map[protoreflect.FullName]protoreflect.MessageDescriptor{
		timestampDesc.FullName(): timestampDesc,
		durationDesc.FullName():  durationDesc,
		anyDesc.FullName():       anyDesc,
	}
	t.Run("empty registries", func(t *testing.T) {
		exerciseWellKnownGraph(
			t,
			new(protoregistry.Types),
			new(protoregistry.Files),
			generatedDescriptors,
		)
	})
	for _, registryMode := range []string{"types only", "files only", "types and files"} {
		t.Run(registryMode, func(t *testing.T) {
			types, files, descriptors := dynamicWellKnownRegistries(t)
			switch registryMode {
			case "types only":
				files = new(protoregistry.Files)
			case "files only":
				types = new(protoregistry.Types)
			}
			exerciseWellKnownGraph(t, types, files, descriptors)
		})
	}
}

func extensionHost(
	t *testing.T,
	path string,
	pkg string,
) (protoreflect.MessageDescriptor, *protoregistry.Files) {
	t.Helper()
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    new(path),
		Package: new(pkg),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Host"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	files := new(protoregistry.Files)
	if err := files.RegisterFile(file); err != nil {
		t.Fatal(err)
	}
	return file.Messages().Get(0), files
}

func extensionDescriptor(
	t *testing.T,
	files *protoregistry.Files,
	host protoreflect.MessageDescriptor,
	path string,
	name string,
	number int32,
) protoreflect.FieldDescriptor {
	t.Helper()
	file, err := protodesc.NewFile(
		extensionFileProto(path, string(host.ParentFile().Package()), name, number, host.ParentFile().Path()),
		files,
	)
	if err != nil {
		t.Fatal(err)
	}
	return file.Extensions().Get(0)
}

func extensionFileProto(
	path string,
	pkg string,
	name string,
	number int32,
	dependency string,
) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       new(path),
		Package:    new(pkg),
		Syntax:     new("proto2"),
		Dependency: []string{dependency},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new(name),
			Extendee: new("." + pkg + ".Host"),
			Number:   new(number),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		}},
	}
}

func TestRuntimeStateRejectsDivergentWellKnownSchema(t *testing.T) {
	fileProto := protodesc.ToFileDescriptorProto(
		timestamppb.File_google_protobuf_timestamp_proto,
	)
	fileProto.MessageType[0].Field[0].Type = descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()
	file, err := protodesc.NewFile(fileProto, nil)
	if err != nil {
		t.Fatal(err)
	}
	types := new(protoregistry.Types)
	if err := types.RegisterMessage(dynamicpb.NewMessageType(file.Messages().Get(0))); err != nil {
		t.Fatal(err)
	}
	if _, err := New(
		goja.New(),
		WithResolver(types),
		WithFiles(new(protoregistry.Files)),
	); err == nil {
		t.Fatal("divergent well-known schema was accepted")
	}
}

func exerciseWellKnownGraph(
	t *testing.T,
	types *protoregistry.Types,
	files *protoregistry.Files,
	expected map[protoreflect.FullName]protoreflect.MessageDescriptor,
) {
	t.Helper()
	runtime := goja.New()
	module, err := New(runtime, WithResolver(types), WithFiles(files))
	if err != nil {
		t.Fatal(err)
	}
	exports := runtime.NewObject()
	if err := module.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("pb", exports); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunString(`
		var Timestamp = pb.messageType("google.protobuf.Timestamp");
		var timestamp = pb.timestampFromMs(1700000000123);
		var duration = pb.durationFromMs(-5500);
		var packed = pb.anyPack(Timestamp, timestamp);
		var unpacked = pb.anyUnpack(packed, Timestamp);
		if (pb.timestampMs(unpacked) !== 1700000000123) throw new Error("timestamp roundtrip");
		if (pb.durationMs(duration) !== -5500) throw new Error("duration roundtrip");
		if (!pb.anyIs(packed, Timestamp)) throw new Error("Any identity");
	`); err != nil {
		t.Fatal(err)
	}
	for name, variable := range map[protoreflect.FullName]string{
		"google.protobuf.Timestamp": "timestamp",
		"google.protobuf.Duration":  "duration",
		"google.protobuf.Any":       "packed",
	} {
		message, err := module.UnwrapMessage(runtime.Get(variable))
		if err != nil {
			t.Fatal(err)
		}
		if message.ProtoReflect().Descriptor() != expected[name] {
			t.Fatalf("%s descriptor is not the runtime canonical identity", name)
		}
		messageType, err := module.TypeResolver().FindMessageByName(name)
		if err != nil {
			t.Fatal(err)
		}
		if messageType.Descriptor() != expected[name] {
			t.Fatalf("%s type resolver returned a different identity", name)
		}
		descriptor, err := module.FindDescriptor(name)
		if err != nil {
			t.Fatal(err)
		}
		if descriptor != expected[name] {
			t.Fatalf("%s file resolver returned a different identity", name)
		}
	}
}

func dynamicWellKnownRegistries(
	t *testing.T,
) (*protoregistry.Types, *protoregistry.Files, map[protoreflect.FullName]protoreflect.MessageDescriptor) {
	t.Helper()
	types := new(protoregistry.Types)
	files := new(protoregistry.Files)
	descriptors := make(map[protoreflect.FullName]protoreflect.MessageDescriptor)
	for _, generatedFile := range []protoreflect.FileDescriptor{
		timestamppb.File_google_protobuf_timestamp_proto,
		durationpb.File_google_protobuf_duration_proto,
		anypb.File_google_protobuf_any_proto,
	} {
		file, err := protodesc.NewFile(protodesc.ToFileDescriptorProto(generatedFile), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := files.RegisterFile(file); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < file.Messages().Len(); index++ {
			descriptor := file.Messages().Get(index)
			if err := types.RegisterMessage(dynamicpb.NewMessageType(descriptor)); err != nil {
				t.Fatal(err)
			}
			descriptors[descriptor.FullName()] = descriptor
		}
	}
	return types, files, descriptors
}

func TestSetupExportsAtomicAndIdempotent(t *testing.T) {
	runtime := goja.New()
	first, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	exports := runtime.NewObject()
	if err := first.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := second.SetupExports(exports); err != nil {
		t.Fatalf("shared-state installation was not idempotent: %v", err)
	}

	conflict := runtime.NewObject()
	if err := conflict.Set("encode", "occupied"); err != nil {
		t.Fatal(err)
	}
	if err := first.SetupExports(conflict); err == nil {
		t.Fatal("expected an export conflict")
	}
	if value := conflict.Get("loadDescriptorSet"); value != nil && !goja.IsUndefined(value) {
		t.Fatal("failed installation left a partial API")
	}
}

func TestGeneratedIdentityAndUnknownFields(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	message := &wrapperspb.StringValue{Value: "original"}
	message.ProtoReflect().SetUnknown([]byte{0x10, 0x2a})
	wrapped, err := module.WrapMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := module.UnwrapMessage(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if unwrapped != message {
		t.Fatal("WrapMessage copied or replaced the generated message")
	}

	exports := runtime.NewObject()
	if err := module.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("pb", exports); err != nil {
		t.Fatal(err)
	}
	wire, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("wire", module.newUint8Array(wire)); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`
		pb.decode(pb.messageType("google.protobuf.StringValue"), wire)
	`)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := module.UnwrapMessage(value)
	if err != nil {
		t.Fatal(err)
	}
	generated, ok := decoded.(*wrapperspb.StringValue)
	if !ok {
		t.Fatalf("decode returned %T, want generated *wrapperspb.StringValue", decoded)
	}
	if !bytes.Equal(generated.ProtoReflect().GetUnknown(), message.ProtoReflect().GetUnknown()) {
		t.Fatal("decode did not preserve unknown fields")
	}
}

func TestWrapMessageCanonicalDescriptorIdentity(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.WrapMessage(nil); err == nil {
		t.Fatal("nil message was accepted")
	}
	var typedNil *wrapperspb.StringValue
	if _, err := module.WrapMessage(typedNil); err == nil {
		t.Fatal("typed-nil message was accepted")
	}
	if _, err := module.WrapMessage(invalidProtoMessage{}); err == nil {
		t.Fatal("invalid reflected message was accepted")
	}
	if _, err := module.WrapMessage(&wrapperspb.StringValue{Value: "canonical"}); err != nil {
		t.Fatalf("canonical generated message was rejected: %v", err)
	}

	foreignFile, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    new("foreign_wrappers.proto"),
		Package: new("google.protobuf"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("StringValue"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     new("different"),
				JsonName: new("different"),
				Number:   proto.Int32(1),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
			}},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	foreign := dynamicpb.NewMessage(foreignFile.Messages().Get(0))
	if _, err := module.WrapMessage(foreign); err == nil {
		t.Fatal("foreign same-name descriptor was accepted")
	}

	localFile := descriptorFile("canonical_dynamic.proto", "canonical", "Dynamic")
	data, err := proto.Marshal(localFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.loadFileDescriptorProtoBytes(data); err != nil {
		t.Fatal(err)
	}
	messageType, err := module.TypeResolver().FindMessageByName("canonical.Dynamic")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.WrapMessage(messageType.New().Interface()); err != nil {
		t.Fatalf("canonical dynamic message was rejected: %v", err)
	}
}

type invalidProtoMessage struct{}

func (invalidProtoMessage) ProtoReflect() protoreflect.Message {
	return nil
}

func TestPrivateBrandsAndReceiverChecks(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var M = pb.messageType("test.SimpleMessage"); var msg = new M()`)
	env.mustFail(t, `pb.encode({_pbMsg: msg._pbMsg})`)
	env.mustFail(t, `msg.get.call({}, "name")`)
	env.mustFail(t, `pb.decode(function(){}, new Uint8Array())`)
}

func TestDescriptorGraphOrderAndAtomicity(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	dependency := descriptorFile("graph_dependency.proto", "graph", "Dependency")
	child := descriptorFile("graph_child.proto", "graph", "Child")
	child.Dependency = []string{dependency.GetName()}
	child.MessageType[0].Field = []*descriptorpb.FieldDescriptorProto{{
		Name:     new("dependency"),
		JsonName: new("dependency"),
		Number:   proto.Int32(1),
		Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		TypeName: new(".graph.Dependency"),
	}}
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{child, dependency}}
	data, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	names, err := module.LoadDescriptorSetBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "graph.Child" || names[1] != "graph.Dependency" {
		t.Fatalf("unexpected deterministic names: %v", names)
	}

	bad := descriptorFile("graph_bad.proto", "graphbad", "Bad")
	bad.Dependency = []string{"missing.proto"}
	good := descriptorFile("graph_good.proto", "graphbad", "Good")
	data, err = proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{good, bad},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.LoadDescriptorSetBytes(data); err == nil {
		t.Fatal("expected unresolved graph to fail")
	}
	if _, err := module.FindDescriptor("graphbad.Good"); err == nil {
		t.Fatal("failed graph installation leaked a valid sibling file")
	}
}

func TestDescriptorDuplicateConflict(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	original := descriptorFile("duplicate.proto", "duplicate", "Original")
	data, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if names, err := module.loadFileDescriptorProtoBytes(data); err != nil || len(names) != 1 {
		t.Fatalf("first load = %v, %v", names, err)
	}
	if names, err := module.loadFileDescriptorProtoBytes(data); err != nil || len(names) != 0 {
		t.Fatalf("idempotent load = %v, %v", names, err)
	}
	divergent := descriptorFile("duplicate.proto", "duplicate", "Changed")
	data, err = proto.Marshal(divergent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.loadFileDescriptorProtoBytes(data); err == nil {
		t.Fatal("divergent duplicate path was accepted")
	}
	if _, err := module.FindDescriptor("duplicate.Original"); err != nil {
		t.Fatal("divergent duplicate mutated the installed snapshot")
	}
}

func TestDescriptorGraphRejectsBaseRegistryShadowing(t *testing.T) {
	t.Run("message type", func(t *testing.T) {
		baseProto := descriptorFile("types_only.proto", "typeshadow", "Message")
		baseFile, err := protodesc.NewFile(baseProto, nil)
		if err != nil {
			t.Fatal(err)
		}
		baseTypes := new(protoregistry.Types)
		baseType := dynamicpb.NewMessageType(baseFile.Messages().Get(0))
		if err := baseTypes.RegisterMessage(baseType); err != nil {
			t.Fatal(err)
		}
		module, err := New(
			goja.New(),
			WithResolver(baseTypes),
			WithFiles(new(protoregistry.Files)),
		)
		if err != nil {
			t.Fatal(err)
		}

		incoming := descriptorFile("local_shadow.proto", "typeshadow", "Message")
		data, err := proto.Marshal(incoming)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.loadFileDescriptorProtoBytes(data); err == nil {
			t.Fatal("local graph shadowed a base message type")
		}
		resolved, err := module.TypeResolver().FindMessageByName("typeshadow.Message")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Descriptor() != baseType.Descriptor() {
			t.Fatal("failed installation replaced the base message identity")
		}
	})

	t.Run("enum value", func(t *testing.T) {
		baseProto := &descriptorpb.FileDescriptorProto{
			Name:    new("base_enum.proto"),
			Package: new("enumshadow"),
			Syntax:  new("proto3"),
			EnumType: []*descriptorpb.EnumDescriptorProto{{
				Name: new("Base"),
				Value: []*descriptorpb.EnumValueDescriptorProto{{
					Name:   new("SHARED"),
					Number: proto.Int32(0),
				}},
			}},
		}
		baseFile, err := protodesc.NewFile(baseProto, nil)
		if err != nil {
			t.Fatal(err)
		}
		baseTypes := new(protoregistry.Types)
		if err := baseTypes.RegisterEnum(dynamicpb.NewEnumType(baseFile.Enums().Get(0))); err != nil {
			t.Fatal(err)
		}
		module, err := New(
			goja.New(),
			WithResolver(baseTypes),
			WithFiles(new(protoregistry.Files)),
		)
		if err != nil {
			t.Fatal(err)
		}

		incoming := &descriptorpb.FileDescriptorProto{
			Name:    new("local_enum.proto"),
			Package: new("enumshadow"),
			Syntax:  new("proto3"),
			EnumType: []*descriptorpb.EnumDescriptorProto{{
				Name: new("Local"),
				Value: []*descriptorpb.EnumValueDescriptorProto{{
					Name:   new("SHARED"),
					Number: proto.Int32(0),
				}},
			}},
		}
		data, err := proto.Marshal(incoming)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := module.loadFileDescriptorProtoBytes(data); err == nil {
			t.Fatal("local graph shadowed a base enum-value symbol")
		}
		if _, err := module.FindDescriptor("enumshadow.Local"); err == nil {
			t.Fatal("rejected enum graph mutated the local snapshot")
		}
	})
}

func TestLoadedExtensionRoundTrip(t *testing.T) {
	runtime := goja.New()
	module, err := New(runtime)
	if err != nil {
		t.Fatal(err)
	}
	file := &descriptorpb.FileDescriptorProto{
		Name:    new("extension.proto"),
		Package: new("extensiontest"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Host"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("note"),
			Number:   proto.Int32(100),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			Extendee: new(".extensiontest.Host"),
			JsonName: new("note"),
		}},
	}
	data, err := proto.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	names, err := module.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("loaded names = %v", names)
	}
	exports := runtime.NewObject()
	if err := module.SetupExports(exports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("pb", exports); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`
		var Host = pb.messageType("extensiontest.Host");
		var host = new Host();
		host.set("extensiontest.note", "value");
		var decoded = pb.decode(Host, pb.encode(host));
		decoded.get("[extensiontest.note]");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "value" {
		t.Fatalf("extension round trip = %q", value.String())
	}
	if _, err := module.TypeResolver().FindExtensionByName("extensiontest.note"); err != nil {
		t.Fatalf("extension is absent from type snapshot: %v", err)
	}
}

func TestLosslessIntegersCollectionsAndIterator(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var All = pb.messageType("test.AllTypes");
		var all = new All();
		all.set("int64_val", "9223372036854775807");
		all.set("uint64_val", 18446744073709551615n);
	`)
	if value := env.run(t, `all.get("int64_val")`); value.Export().(*big.Int).String() != "9223372036854775807" {
		t.Fatalf("int64 value = %s", value.String())
	}
	env.mustFail(t, `all.set("int64_val", 9007199254740992)`)

	env.run(t, `
		var Repeated = pb.messageType("test.RepeatedMessage");
		var repeated = new Repeated();
		repeated.set("numbers", [1, 2]);
	`)
	env.mustFail(t, `repeated.set("numbers", [3, 9999999999])`)
	if value := env.run(t, `repeated.get("numbers").get(0) === 1 && repeated.get("numbers").length === 2`); !value.ToBoolean() {
		t.Fatal("failed repeated replacement mutated the original value")
	}

	env.run(t, `
		var Mapped = pb.messageType("test.MapMessage");
		var mapped = new Mapped();
		mapped.set("counts", {old: 1});
	`)
	env.mustFail(t, `mapped.set("counts", {new: 9999999999})`)
	if value := env.run(t, `mapped.get("counts").get("old") === 1 && mapped.get("counts").size === 1`); !value.ToBoolean() {
		t.Fatal("failed map replacement mutated the original value")
	}
	env.run(t, `mapped.set("tags", {a: "A", b: "B"})`)
	if value := env.run(t, `Array.from(mapped.get("tags")).length`); value.ToInteger() != 2 {
		t.Fatal("map wrapper is not a real iterable")
	}
}

func descriptorFile(path, pkg, message string) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    new(path),
		Package: new(pkg),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new(message),
		}},
	}
}
