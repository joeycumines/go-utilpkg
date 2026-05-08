package gojaprotobuf

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Module provides Protocol Buffers support for a [goja.Runtime].
// Every Module bound to the same runtime shares one descriptor, type,
// extension, message, and wrapper identity.
//
// Type resolution checks the shared runtime-local descriptor graph first,
// then the immutable base snapshot captured by the runtime's first Module.
type Module struct {
	runtime *goja.Runtime
	state   *runtimeState
}

// New creates a new [Module] bound to the given [goja.Runtime].
//
// New panics if runtime is nil, an option is nil or invalid, or a later
// construction for the same runtime provides different source registry
// instances. These are static construction contract violations.
//
// The first construction snapshots the configured type and file registry
// membership. Later construction for the same runtime reuses that snapshot.
// New returns an error when registry contents cannot form a valid descriptor
// graph or live Goja state prevents construction.
func New(runtime *goja.Runtime, opts ...ModuleOption) (*Module, error) {
	if runtime == nil {
		panic("gojaprotobuf: runtime must not be nil")
	}

	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Errorf("gojaprotobuf: %w", err))
	}
	return newModule(runtime, cfg)
}

func newModule(
	runtime *goja.Runtime,
	cfg *moduleConfig,
) (*Module, error) {
	state, err := acquireRuntimeState(runtime, cfg.resolver, cfg.files)
	if err != nil {
		if errors.Is(err, errBaseRegistryIdentity) {
			panic(fmt.Errorf("gojaprotobuf: %w", err))
		}
		return nil, fmt.Errorf("gojaprotobuf: %w", err)
	}
	return &Module{runtime: runtime, state: state}, nil
}

// OwnsRuntime reports whether candidate owns this module's canonical
// protobuf state. It does not access Goja state.
func (m *Module) OwnsRuntime(candidate *goja.Runtime) bool {
	return m != nil && m.state != nil && candidate != nil && m.state.runtime == candidate
}

// FindDescriptor looks up a descriptor by its fully-qualified name,
// searching the module's shared runtime-local graph first, then its immutable
// base snapshot. This is useful for resolving service, message, or
// enum descriptors by name from Go code that works alongside the JS
// module.
func (m *Module) FindDescriptor(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	return m.fileResolver().FindDescriptorByName(name)
}

// WrapMessage wraps a canonical [proto.Message] as a JavaScript object.
// The message descriptor must be the exact descriptor owned by this runtime's
// type registry; a foreign same-name schema is rejected without mutation.
func (m *Module) WrapMessage(msg proto.Message) (*goja.Object, error) {
	if _, err := m.canonicalMessage(msg); err != nil {
		return nil, err
	}
	return m.wrapCanonicalMessage(msg), nil
}

// UnwrapMessage extracts a [proto.Message] from a JavaScript value
// that was created by this module's [WrapMessage] or a messageType
// constructor. Returns an error if the value is not a valid protobuf
// message wrapper.
func (m *Module) UnwrapMessage(val goja.Value) (proto.Message, error) {
	return m.unwrapMessage(val)
}

// SetupExports wires the module's JS API onto the given exports object.
// This is equivalent to the setup performed by [Enable] but allows
// external consumers to configure exports without the require() mechanism.
func (m *Module) SetupExports(exports *goja.Object) error {
	if m == nil || m.state == nil {
		return fmt.Errorf("gojaprotobuf: module is nil")
	}
	if exports == nil {
		return fmt.Errorf("gojaprotobuf: exports object is nil")
	}
	return m.setupExports(exports)
}

// LoadDescriptorSetBytes parses a serialized
// [google.golang.org/protobuf/types/descriptorpb.FileDescriptorSet]
// and atomically registers all contained files and types into the canonical
// graph shared by every module bound to this runtime.
// Returns the list of registered fully-qualified type names.
func (m *Module) LoadDescriptorSetBytes(data []byte) ([]string, error) {
	return m.loadDescriptorSetBytes(data)
}

// FileResolver returns a [protodesc.Resolver] that checks the module's shared
// runtime-local graph first, then its immutable base snapshot. This is useful
// for integrating with services that need
// descriptor resolution (e.g., gRPC reflection).
func (m *Module) FileResolver() interface {
	FindFileByPath(string) (protoreflect.FileDescriptor, error)
	FindDescriptorByName(protoreflect.FullName) (protoreflect.Descriptor, error)
} {
	return m.fileResolver()
}

// TypeResolver returns a resolver that checks the module's shared
// runtime-local types first, then its immutable base snapshot. It supports
// extension enumeration for gRPC reflection and satisfies the Resolver
// interface required by [protojson.MarshalOptions] and
// [protojson.UnmarshalOptions] for expanding google.protobuf.Any messages.
func (m *Module) TypeResolver() interface {
	FindMessageByName(protoreflect.FullName) (protoreflect.MessageType, error)
	FindMessageByURL(string) (protoreflect.MessageType, error)
	FindExtensionByName(protoreflect.FullName) (protoreflect.ExtensionType, error)
	FindExtensionByNumber(protoreflect.FullName, protoreflect.FieldNumber) (protoreflect.ExtensionType, error)
	RangeExtensionsByMessage(protoreflect.FullName, func(protoreflect.ExtensionType) bool)
} {
	return m.typeResolver()
}

// setupExports wires the module's JS API onto the given exports object.
func (m *Module) setupExports(exports *goja.Object) error {
	return m.installExports(exports, map[string]any{
		"loadDescriptorSet":       m.jsLoadDescriptorSet,
		"loadFileDescriptorProto": m.jsLoadFileDescriptorProto,
		"messageType":             m.jsMessageType,
		"enumType":                m.jsEnumType,
		"encode":                  m.jsEncode,
		"decode":                  m.jsDecode,
		"toJSON":                  m.jsToJSON,
		"fromJSON":                m.jsFromJSON,
		"equals":                  m.jsEquals,
		"clone":                   m.jsClone,
		"isMessage":               m.jsIsMessage,
		"isFieldSet":              m.jsIsFieldSet,
		"clearField":              m.jsClearField,
		"timestampNow":            m.jsTimestampNow,
		"timestampFromDate":       m.jsTimestampFromDate,
		"timestampDate":           m.jsTimestampDate,
		"timestampFromMs":         m.jsTimestampFromMs,
		"timestampMs":             m.jsTimestampMs,
		"durationFromMs":          m.jsDurationFromMs,
		"durationMs":              m.jsDurationMs,
		"anyPack":                 m.jsAnyPack,
		"anyUnpack":               m.jsAnyUnpack,
		"anyIs":                   m.jsAnyIs,
	})
}
