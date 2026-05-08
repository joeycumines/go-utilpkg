package gojagrpc

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMessageDescriptorRejectsForeignSameSchema(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	expected := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")
	foreignFile, err := protodesc.NewFile(testGrpcFileDescriptorProto(), nil)
	if err != nil {
		t.Fatal(err)
	}
	foreign := dynamicpb.NewMessage(foreignFile.Messages().ByName("EchoRequest"))
	err = validateMessageDescriptor(foreign, expected)
	if err == nil || !strings.Contains(err.Error(), "non-canonical descriptor") {
		t.Fatalf("same-schema foreign descriptor error = %v, want canonical-identity rejection", err)
	}
}

func TestWrapMessagePreservesCanonicalGeneratedIdentity(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	message := wrapperspb.String("generated")
	unknown := []byte{0xa0, 0x06, 0x01}
	message.ProtoReflect().SetUnknown(append([]byte(nil), unknown...))
	wrapped, err := env.grpcMod.wrapMessage(message, message.ProtoReflect().Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := env.pbMod.UnwrapMessage(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	generated, ok := unwrapped.(*wrapperspb.StringValue)
	if !ok {
		t.Fatalf("wrapped generated identity = %T, want *wrapperspb.StringValue", unwrapped)
	}
	if generated == message {
		t.Fatal("wrapMessage exposed the transport-owned message without cloning")
	}
	if generated.Value != "generated" || !bytes.Equal(generated.ProtoReflect().GetUnknown(), unknown) {
		t.Fatalf("generated wrapper lost value or unknown fields: %#v", generated)
	}
}
