package gojaprotobuf

import (
	"fmt"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// combinedFileResolver resolves file descriptors through staged local
// membership, the immutable base registry, and reachable base graph files.
// It implements [protodesc.Resolver].
type combinedFileResolver struct {
	local  *protoregistry.Files
	global *protoregistry.Files
	graph  *descriptorGraph
}

func (r *combinedFileResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	fd, err := r.local.FindFileByPath(path)
	if err == nil {
		return fd, nil
	}
	if fd, err = r.global.FindFileByPath(path); err == nil {
		return fd, nil
	}
	if r.graph != nil {
		if fd, ok := r.graph.files[path]; ok {
			return fd, nil
		}
	}
	return nil, protoregistry.NotFound
}

func (r *combinedFileResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	d, err := r.local.FindDescriptorByName(name)
	if err == nil {
		return d, nil
	}
	if d, err = r.global.FindDescriptorByName(name); err == nil {
		return d, nil
	}
	if r.graph != nil {
		if d, ok := r.graph.symbols[name]; ok {
			return d, nil
		}
	}
	return nil, protoregistry.NotFound
}

// combinedTypeResolver resolves message and extension types by checking
// the local registry first, then falling back to the configured global
// registry. It satisfies the Resolver interface required by
// [protojson.MarshalOptions] and [protojson.UnmarshalOptions].
type combinedTypeResolver struct {
	local  *protoregistry.Types
	global *protoregistry.Types
}

func (r *combinedTypeResolver) FindMessageByName(name protoreflect.FullName) (protoreflect.MessageType, error) {
	mt, err := r.local.FindMessageByName(name)
	if err == nil {
		return mt, nil
	}
	return r.global.FindMessageByName(name)
}

func (r *combinedTypeResolver) FindMessageByURL(url string) (protoreflect.MessageType, error) {
	mt, err := r.local.FindMessageByURL(url)
	if err == nil {
		return mt, nil
	}
	return r.global.FindMessageByURL(url)
}

func (r *combinedTypeResolver) FindExtensionByName(field protoreflect.FullName) (protoreflect.ExtensionType, error) {
	xt, err := r.local.FindExtensionByName(field)
	if err == nil {
		return xt, nil
	}
	return r.global.FindExtensionByName(field)
}

func (r *combinedTypeResolver) FindExtensionByNumber(message protoreflect.FullName, field protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	xt, err := r.local.FindExtensionByNumber(message, field)
	if err == nil {
		return xt, nil
	}
	return r.global.FindExtensionByNumber(message, field)
}

// extractBytes extracts a []byte from a JS value that represents binary
// data. It accepts Uint8Array, ArrayBuffer, or any value that exports
// as []byte.
func (m *Module) extractBytes(val goja.Value) ([]byte, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, fmt.Errorf("expected Uint8Array or ArrayBuffer, got null/undefined")
	}

	exported := val.Export()

	// goja exports ArrayBuffer as goja.ArrayBuffer.
	if ab, ok := exported.(goja.ArrayBuffer); ok {
		return ab.Bytes(), nil
	}

	// goja exports Uint8Array as []byte.
	if b, ok := exported.([]byte); ok {
		return b, nil
	}

	// Try ExportTo as a fallback.
	var b []byte
	if err := m.runtime.ExportTo(val, &b); err == nil {
		return b, nil
	}

	return nil, fmt.Errorf("expected Uint8Array or ArrayBuffer, got %T", exported)
}

// newUint8Array creates a JavaScript Uint8Array from a Go byte slice.
func (m *Module) newUint8Array(data []byte) goja.Value {
	ab := m.runtime.NewArrayBuffer(data)
	uint8ArrayCtor := m.runtime.Get("Uint8Array")
	if uint8ArrayCtor == nil || goja.IsUndefined(uint8ArrayCtor) {
		// Fallback: return ArrayBuffer directly.
		return m.runtime.ToValue(ab)
	}
	result, err := m.runtime.New(uint8ArrayCtor, m.runtime.ToValue(ab))
	if err != nil {
		// Fallback: return ArrayBuffer.
		return m.runtime.ToValue(ab)
	}
	return result
}

type messageTypeHolder struct {
	state       *runtimeState
	messageType protoreflect.MessageType
}

// messageDescHolder is retained as an unexported source-compatibility alias for
// package tests and does not participate in constructor authentication.
type messageDescHolder = messageTypeHolder

// extractMessageType extracts the privately branded canonical message type
// from a constructor created by this runtime's protobuf state.
func (m *Module) extractMessageType(val goja.Value) (protoreflect.MessageType, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, fmt.Errorf("expected message type constructor, got null/undefined")
	}

	obj := val.ToObject(m.runtime)
	if obj == nil {
		return nil, fmt.Errorf("expected message type constructor, got non-object")
	}

	holderVal, found, err := m.state.constructors.load(obj)
	if err != nil {
		return nil, fmt.Errorf("message type constructor runtime mismatch: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("not a protobuf message type constructor")
	}

	holder, ok := holderVal.Export().(*messageTypeHolder)
	if !ok || holder == nil || holder.state != m.state || holder.messageType == nil {
		return nil, fmt.Errorf("not a protobuf message type constructor")
	}

	return holder.messageType, nil
}

func (m *Module) extractMessageDesc(val goja.Value) (protoreflect.MessageDescriptor, error) {
	messageType, err := m.extractMessageType(val)
	if err != nil {
		return nil, err
	}
	return messageType.Descriptor(), nil
}

func (m *Module) typeResolver() stateTypeResolver {
	return stateTypeResolver{state: m.state}
}

func (m *Module) fileResolver() stateFileResolver {
	return stateFileResolver{state: m.state}
}
