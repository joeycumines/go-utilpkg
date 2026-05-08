package inprocgrpc

import (
	"testing"

	"google.golang.org/grpc/encoding"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type nilCodecV1 struct{}

func (*nilCodecV1) Marshal(any) ([]byte, error) { panic("unexpected call") }
func (*nilCodecV1) Unmarshal([]byte, any) error { panic("unexpected call") }
func (*nilCodecV1) Name() string                { panic("unexpected call") }

type nilCodecV2 struct{}

func (*nilCodecV2) Marshal(any) (mem.BufferSlice, error) { panic("unexpected call") }
func (*nilCodecV2) Unmarshal(mem.BufferSlice, any) error { panic("unexpected call") }
func (*nilCodecV2) Name() string                         { panic("unexpected call") }

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

func TestClonerConstructorsRejectNil(t *testing.T) {
	var (
		codecV1 *nilCodecV1
		codecV2 *nilCodecV2
	)
	tests := []struct {
		name string
		fn   func()
	}{
		{name: "clone function", fn: func() { CloneFunc(nil) }},
		{name: "copy function", fn: func() { CopyFunc(nil) }},
		{name: "codec v1", fn: func() { CodecCloner(codecV1) }},
		{name: "codec v2", fn: func() { CodecClonerV2(codecV2) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertPanicContains(t, "must not be nil", test.fn)
		})
	}
}

func TestIsNil(t *testing.T) {
	var p *wrapperspb.StringValue
	var channel chan struct{}
	var function func()
	var values map[string]string
	var bytes []byte
	for name, value := range map[string]any{
		"nil":      nil,
		"pointer":  p,
		"channel":  channel,
		"function": function,
		"map":      values,
		"slice":    bytes,
	} {
		t.Run(name, func(t *testing.T) {
			if !isNil(value) {
				t.Fatal("typed nil value was not recognized")
			}
		})
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
