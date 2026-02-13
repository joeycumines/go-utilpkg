package inprocgrpc

import (
	"fmt"
	"reflect"

	"google.golang.org/grpc/encoding"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/protobuf/proto"
)

// getCodecV2 is the codec lookup function used by ProtoCloner for non-proto
// message fallback. Package-level var to allow test override for coverage of
// the unreachable-in-normal-operation fallback error paths.
var getCodecV2 = encoding.GetCodecV2

// Cloner is used to copy messages between client and server within the
// in-process channel. Because both sides share the same address space,
// messages must be isolated to prevent concurrent mutation.
type Cloner interface {
	// Copy copies the contents of in into out. Both must be the same
	// concrete type (typically a pointer to a proto message).
	Copy(out, in any) error

	// Clone creates a deep copy of the given message.
	Clone(any) (any, error)
}

// ProtoCloner is the default Cloner that handles proto.Message instances.
// It uses proto.Clone for cloning and proto.Merge for copying.
//
// For non-proto messages, it falls back to the registered gRPC "proto" codec.
type ProtoCloner struct{}

// Copy copies in to out using proto.Merge (after reset) for proto messages,
// or falls back to the gRPC proto codec for other types.
func (ProtoCloner) Copy(out, in any) error {
	inMsg, inOk := in.(proto.Message)
	outMsg, outOk := out.(proto.Message)
	if inOk && outOk {
		proto.Reset(outMsg)
		proto.Merge(outMsg, inMsg)
		return nil
	}
	codec := getCodecV2(grpcproto.Name)
	if codec != nil {
		return codecClonerV2{codec: codec}.Copy(out, in)
	}
	return fmt.Errorf("inprocgrpc: no codec found for non-proto message copying")
}

// Clone creates a deep copy of in using proto.Clone for proto messages,
// or falls back to the gRPC proto codec for other types.
func (ProtoCloner) Clone(in any) (any, error) {
	if msg, ok := in.(proto.Message); ok {
		return proto.Clone(msg), nil
	}
	codec := getCodecV2(grpcproto.Name)
	if codec != nil {
		return codecClonerV2{codec: codec}.Clone(in)
	}
	return nil, fmt.Errorf("inprocgrpc: no codec found for non-proto message cloning")
}

// CloneFunc creates a Cloner from a clone function.
// Copy is implemented by cloning, then shallow-copying via reflection.
func CloneFunc(fn func(any) (any, error)) Cloner {
	return funcCloner{
		cloneFn: fn,
		copyFn: func(out, in any) error {
			cloned, err := fn(in)
			if err != nil {
				return err
			}
			reflect.ValueOf(out).Elem().Set(reflect.ValueOf(cloned).Elem())
			return nil
		},
	}
}

// CopyFunc creates a Cloner from a copy function.
// Clone is implemented by creating a new zero value and copying into it.
func CopyFunc(fn func(out, in any) error) Cloner {
	return funcCloner{
		cloneFn: func(in any) (any, error) {
			out := reflect.New(reflect.TypeOf(in).Elem()).Interface()
			if err := fn(out, in); err != nil {
				return nil, err
			}
			return out, nil
		},
		copyFn: fn,
	}
}

// CodecCloner creates a Cloner that uses a gRPC codec (v1) for cloning.
// Messages are marshaled and then unmarshaled - a full roundtrip.
func CodecCloner(codec encoding.Codec) Cloner {
	return codecClonerV1{codec: codec}
}

// CodecClonerV2 creates a Cloner that uses a gRPC CodecV2 for cloning.
// Messages are marshaled and then unmarshaled - a full roundtrip.
func CodecClonerV2(codec encoding.CodecV2) Cloner {
	return codecClonerV2{codec: codec}
}

type funcCloner struct {
	cloneFn func(any) (any, error)
	copyFn  func(out, in any) error
}

func (c funcCloner) Clone(in any) (any, error) { return c.cloneFn(in) }
func (c funcCloner) Copy(out, in any) error    { return c.copyFn(out, in) }

type codecClonerV1 struct {
	codec encoding.Codec
}

func (c codecClonerV1) Copy(out, in any) error {
	data, err := c.codec.Marshal(in)
	if err != nil {
		return err
	}
	return c.codec.Unmarshal(data, out)
}

func (c codecClonerV1) Clone(in any) (any, error) {
	out := reflect.New(reflect.TypeOf(in).Elem()).Interface()
	if err := c.Copy(out, in); err != nil {
		return nil, err
	}
	return out, nil
}

type codecClonerV2 struct {
	codec encoding.CodecV2
}

func (c codecClonerV2) Copy(out, in any) error {
	data, err := c.codec.Marshal(in)
	if err != nil {
		return err
	}
	return c.codec.Unmarshal(data, out)
}

func (c codecClonerV2) Clone(in any) (any, error) {
	out := reflect.New(reflect.TypeOf(in).Elem()).Interface()
	if err := c.Copy(out, in); err != nil {
		return nil, err
	}
	return out, nil
}
