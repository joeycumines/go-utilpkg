package gojaprotobuf

import (
	"fmt"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// combinedFileResolver resolves file descriptors by checking the local
// registry first, then falling back to the configured global registry.
// It implements [protodesc.Resolver].
type combinedFileResolver struct {
	local  *protoregistry.Files
	global *protoregistry.Files
}

func (r *combinedFileResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	fd, err := r.local.FindFileByPath(path)
	if err == nil {
		return fd, nil
	}
	return r.global.FindFileByPath(path)
}

func (r *combinedFileResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	d, err := r.local.FindDescriptorByName(name)
	if err == nil {
		return d, nil
	}
	return r.global.FindDescriptorByName(name)
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

// messageDescHolder wraps a [protoreflect.MessageDescriptor] for storage
// in a goja object property. This ensures the descriptor survives the
// Export/import cycle through the goja runtime.
type messageDescHolder struct {
	desc protoreflect.MessageDescriptor
}

// extractMessageDesc extracts a [protoreflect.MessageDescriptor] from a
// constructor value. The constructor must have been created by
// [Module.jsMessageType].
func (m *Module) extractMessageDesc(val goja.Value) (protoreflect.MessageDescriptor, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, fmt.Errorf("expected message type constructor, got null/undefined")
	}

	obj := val.ToObject(m.runtime)
	if obj == nil {
		return nil, fmt.Errorf("expected message type constructor, got non-object")
	}

	holderVal := obj.Get("_pbMsgDesc")
	if holderVal == nil || goja.IsUndefined(holderVal) {
		return nil, fmt.Errorf("not a protobuf message type constructor")
	}

	holder, ok := holderVal.Export().(*messageDescHolder)
	if !ok || holder == nil {
		return nil, fmt.Errorf("not a protobuf message type constructor")
	}

	return holder.desc, nil
}

// typeResolver returns a combined type resolver that checks local types
// first, then falls back to the module's configured resolver.
func (m *Module) typeResolver() *combinedTypeResolver {
	return &combinedTypeResolver{
		local:  m.localTypes,
		global: m.resolver,
	}
}

// fileResolver returns a combined file resolver that checks local files
// first, then falls back to the module's configured files.
func (m *Module) fileResolver() *combinedFileResolver {
	return &combinedFileResolver{
		local:  m.localFiles,
		global: m.files,
	}
}
