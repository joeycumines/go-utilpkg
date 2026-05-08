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

// Cloner is used concurrently across Channel RPCs and by the permitted sender
// and receiver of each stream. Implementations must be concurrency-safe.
// Because both sides share the same address space, messages must be isolated
// to prevent concurrent mutation.
type Cloner interface {
	// Copy copies the contents of in into out. Both must be the same
	// concrete type (typically a pointer to a proto message).
	Copy(out, in any) error

	// Clone creates a deep copy of the given message.
	Clone(any) (any, error)
}

type cloneCallResult struct {
	value any
	err   error
}

func cloneMessageSafe(
	operation string,
	cloner Cloner,
	message any,
) cloneCallResult {
	result := make(chan cloneCallResult, 1)
	go func() {
		returned := false
		output := cloneCallResult{}
		defer func() {
			_ = recover()
			if !returned {
				output = cloneCallResult{
					err: internalFailureError(operation),
				}
			}
			result <- output
		}()
		output.value, output.err = cloner.Clone(message)
		if output.err != nil {
			output.err = cloneError(operation, output.err)
		}
		returned = true
	}()
	return <-result
}

func copyMessageSafe(
	operation string,
	cloner Cloner,
	target any,
	source any,
) error {
	result := make(chan error, 1)
	go func() {
		returned := false
		var err error
		defer func() {
			_ = recover()
			if !returned {
				err = internalFailureError(operation)
			}
			result <- err
		}()
		err = cloner.Copy(target, source)
		if err != nil {
			err = cloneError(operation, err)
		}
		returned = true
	}()
	return <-result
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
// Derived Copy requires non-nil pointers with the same element type.
// CloneFunc panics if fn is nil.
func CloneFunc(fn func(any) (any, error)) Cloner {
	if fn == nil {
		panic("inprocgrpc: clone function must not be nil")
	}
	return funcCloner{
		cloneFn: fn,
		copyFn: func(out, in any) error {
			outValue, outType, err := clonePointer(out, "copy output")
			if err != nil {
				return err
			}
			cloned, err := fn(in)
			if err != nil {
				return err
			}
			clonedValue, clonedType, err := clonePointer(
				cloned,
				"clone result",
			)
			if err != nil {
				return err
			}
			if clonedType != outType {
				return fmt.Errorf(
					"inprocgrpc: clone result element type %s does not match copy output %s",
					clonedType,
					outType,
				)
			}
			outValue.Set(clonedValue)
			return nil
		},
	}
}

// CopyFunc creates a Cloner from a copy function.
// Clone is implemented by creating a new zero value and copying into it.
// Derived Clone requires a non-nil pointer input.
// CopyFunc panics if fn is nil.
func CopyFunc(fn func(out, in any) error) Cloner {
	if fn == nil {
		panic("inprocgrpc: copy function must not be nil")
	}
	return funcCloner{
		cloneFn: func(in any) (any, error) {
			_, elementType, err := clonePointer(in, "clone input")
			if err != nil {
				return nil, err
			}
			out := reflect.New(elementType).Interface()
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
// CodecCloner panics if codec is nil, including a typed nil.
func CodecCloner(codec encoding.Codec) Cloner {
	if isNil(codec) {
		panic("inprocgrpc: codec must not be nil")
	}
	return codecClonerV1{codec: codec}
}

// CodecClonerV2 creates a Cloner that uses a gRPC CodecV2 for cloning.
// Messages are marshaled and then unmarshaled - a full roundtrip.
// CodecClonerV2 panics if codec is nil, including a typed nil.
func CodecClonerV2(codec encoding.CodecV2) Cloner {
	if isNil(codec) {
		panic("inprocgrpc: codec must not be nil")
	}
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
	_, elementType, err := clonePointer(in, "clone input")
	if err != nil {
		return nil, err
	}
	out := reflect.New(elementType).Interface()
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
	_, elementType, err := clonePointer(in, "clone input")
	if err != nil {
		return nil, err
	}
	out := reflect.New(elementType).Interface()
	if err := c.Copy(out, in); err != nil {
		return nil, err
	}
	return out, nil
}

func clonePointer(
	value any,
	subject string,
) (reflect.Value, reflect.Type, error) {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() ||
		reflected.Kind() != reflect.Pointer ||
		reflected.IsNil() {
		return reflect.Value{}, nil, fmt.Errorf(
			"inprocgrpc: %s must be a non-nil pointer",
			subject,
		)
	}
	return reflected.Elem(), reflected.Type().Elem(), nil
}
