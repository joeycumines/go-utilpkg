package gojagrpc

import (
	"context"
	"slices"
	"testing"

	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestReflectionEnumeratesRuntimeOnlyExtensions(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("runtime_only_extension.proto"),
		Package: proto.String("runtimeonly"),
		Syntax:  proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Host"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     proto.String("note"),
			Number:   proto.Int32(123),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			Extendee: proto.String(".runtimeonly.Host"),
		}},
	}
	data, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{file},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.pbMod.LoadDescriptorSetBytes(data); err != nil {
		t.Fatalf("load runtime-only extension: %v", err)
	}
	if _, err := protoregistry.GlobalTypes.FindExtensionByName(
		protoreflect.FullName("runtimeonly.note"),
	); err == nil {
		t.Fatal("runtime-only extension unexpectedly exists in GlobalTypes")
	}
	if _, err := env.pbMod.TypeResolver().FindExtensionByName(
		protoreflect.FullName("runtimeonly.note"),
	); err != nil {
		t.Fatalf("shared TypeResolver extension lookup: %v", err)
	}
	if err := env.grpcMod.EnableReflection(); err != nil {
		t.Fatalf("EnableReflection: %v", err)
	}

	stop := withLoopRunning(t, env, defaultTimeout)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(
		env.channel,
	).ServerReflectionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.CloseSend()
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_AllExtensionNumbersOfType{
			AllExtensionNumbersOfType: "runtimeonly.Host",
		},
	}); err != nil {
		t.Fatal(err)
	}
	response, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	extensions := response.GetAllExtensionNumbersResponse()
	if extensions == nil {
		t.Fatalf("reflection response = %T, want extension numbers", response.GetMessageResponse())
	}
	if extensions.GetBaseTypeName() != "runtimeonly.Host" ||
		!slices.Equal(extensions.GetExtensionNumber(), []int32{123}) {
		t.Fatalf(
			"extension response = base:%q numbers:%v",
			extensions.GetBaseTypeName(),
			extensions.GetExtensionNumber(),
		)
	}
}
