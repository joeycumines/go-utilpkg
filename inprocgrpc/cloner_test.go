package inprocgrpc

import (
	"testing"

	"google.golang.org/grpc/encoding"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestProtoCloner_Clone(t *testing.T) {
	c := ProtoCloner{}
	orig := &wrapperspb.StringValue{Value: "hello"}
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatal(err)
	}
	msg := cloned.(*wrapperspb.StringValue)
	if msg.GetValue() != "hello" {
		t.Errorf("got %q", msg.GetValue())
	}

	// Verify independence
	msg.Value = "modified"
	if orig.GetValue() != "hello" {
		t.Error("original mutated")
	}
}

func TestProtoCloner_Copy(t *testing.T) {
	c := ProtoCloner{}
	src := &wrapperspb.StringValue{Value: "hello"}
	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if dst.GetValue() != "hello" {
		t.Errorf("got %q", dst.GetValue())
	}

	// Verify independence
	dst.Value = "modified"
	if src.GetValue() != "hello" {
		t.Error("source mutated")
	}
}

func TestCloneFunc(t *testing.T) {
	c := CloneFunc(func(in any) (any, error) {
		return proto.Clone(in.(proto.Message)), nil
	})

	orig := &wrapperspb.StringValue{Value: "hello"}

	// Test Clone
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.(*wrapperspb.StringValue).GetValue() != "hello" {
		t.Error("clone failed")
	}

	// Test Copy (derived from Clone)
	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, orig); err != nil {
		t.Fatal(err)
	}
	if dst.GetValue() != "hello" {
		t.Error("copy failed")
	}

	// Verify independence
	cloned.(*wrapperspb.StringValue).Value = "x"
	dst.Value = "y"
	if orig.GetValue() != "hello" {
		t.Error("original mutated")
	}
}

func TestCopyFunc(t *testing.T) {
	c := CopyFunc(func(out, in any) error {
		proto.Reset(out.(proto.Message))
		proto.Merge(out.(proto.Message), in.(proto.Message))
		return nil
	})

	orig := &wrapperspb.StringValue{Value: "hello"}

	// Test Copy
	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, orig); err != nil {
		t.Fatal(err)
	}
	if dst.GetValue() != "hello" {
		t.Error("copy failed")
	}

	// Test Clone (derived from Copy)
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatal(err)
	}
	if cloned.(*wrapperspb.StringValue).GetValue() != "hello" {
		t.Error("clone failed")
	}

	// Verify independence
	dst.Value = "x"
	cloned.(*wrapperspb.StringValue).Value = "y"
	if orig.GetValue() != "hello" {
		t.Error("original mutated")
	}
}

func TestIsNil(t *testing.T) {
	if !isNil(nil) {
		t.Error("nil should be nil")
	}

	var p *wrapperspb.StringValue
	if !isNil(p) {
		t.Error("typed nil should be nil")
	}

	if isNil(&wrapperspb.StringValue{}) {
		t.Error("non-nil should not be nil")
	}

	if isNil("string") {
		t.Error("non-pointer should not be nil")
	}
}

func TestCodecClonerV2_Clone(t *testing.T) {
	codec := encoding.GetCodecV2(grpcproto.Name)
	if codec == nil {
		t.Skip("proto codec not available")
	}
	c := CodecClonerV2(codec)
	orig := &wrapperspb.StringValue{Value: "hello"}
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatal(err)
	}
	msg := cloned.(*wrapperspb.StringValue)
	if msg.GetValue() != "hello" {
		t.Errorf("got %q", msg.GetValue())
	}
	msg.Value = "modified"
	if orig.GetValue() != "hello" {
		t.Error("original was mutated")
	}
}

func TestCodecClonerV2_Copy(t *testing.T) {
	codec := encoding.GetCodecV2(grpcproto.Name)
	if codec == nil {
		t.Skip("proto codec not available")
	}
	c := CodecClonerV2(codec)
	src := &wrapperspb.StringValue{Value: "hello"}
	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if dst.GetValue() != "hello" {
		t.Errorf("got %q", dst.GetValue())
	}
	dst.Value = "modified"
	if src.GetValue() != "hello" {
		t.Error("source was mutated")
	}
}

func TestCodecClonerV1_Clone(t *testing.T) {
	codec := encoding.GetCodec(grpcproto.Name)
	if codec == nil {
		t.Skip("proto codec v1 not available")
	}
	c := CodecCloner(codec)
	orig := &wrapperspb.StringValue{Value: "hello"}
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatal(err)
	}
	msg := cloned.(*wrapperspb.StringValue)
	if msg.GetValue() != "hello" {
		t.Errorf("got %q", msg.GetValue())
	}
}

func TestCodecClonerV1_Copy(t *testing.T) {
	codec := encoding.GetCodec(grpcproto.Name)
	if codec == nil {
		t.Skip("proto codec v1 not available")
	}
	c := CodecCloner(codec)
	src := &wrapperspb.StringValue{Value: "world"}
	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if dst.GetValue() != "world" {
		t.Errorf("got %q", dst.GetValue())
	}
}

func TestProtoCloner_CopyFallbackToCodec(t *testing.T) {
	// ProtoCloner.Copy with both operands being proto messages should work
	// directly without codec fallback
	c := ProtoCloner{}
	src := &wrapperspb.Int64Value{Value: 42}
	dst := new(wrapperspb.Int64Value)
	if err := c.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if dst.GetValue() != 42 {
		t.Errorf("got %d", dst.GetValue())
	}
}

func TestProtoCloner_CloneDifferentTypes(t *testing.T) {
	c := ProtoCloner{}
	orig := &wrapperspb.BoolValue{Value: true}
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatal(err)
	}
	msg := cloned.(*wrapperspb.BoolValue)
	if !msg.GetValue() {
		t.Error("clone lost the value")
	}
}
