package gojaprotobuf

import (
	"fmt"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Module provides Protocol Buffers support for a [goja.Runtime].
// Each Module instance is bound to a single runtime and maintains
// its own local type and file registries for descriptors loaded
// via [loadDescriptorSet] and [loadFileDescriptorProto].
//
// Type resolution checks the local registries first, then falls
// back to the configured resolver (default: [protoregistry.GlobalTypes]).
type Module struct {
	runtime    *goja.Runtime
	resolver   *protoregistry.Types
	files      *protoregistry.Files
	localTypes *protoregistry.Types
	localFiles *protoregistry.Files
}

// New creates a new [Module] bound to the given [goja.Runtime].
//
// New panics if runtime is nil, as this is a programming error
// (invariant violation). It returns an error if option validation
// fails.
func New(runtime *goja.Runtime, opts ...Option) (*Module, error) {
	if runtime == nil {
		panic("gojaprotobuf: runtime must not be nil")
	}

	cfg, err := resolveOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("gojaprotobuf: %w", err)
	}

	resolver := cfg.resolver
	if resolver == nil {
		resolver = protoregistry.GlobalTypes
	}

	files := cfg.files
	if files == nil {
		files = protoregistry.GlobalFiles
	}

	return &Module{
		runtime:    runtime,
		resolver:   resolver,
		files:      files,
		localTypes: new(protoregistry.Types),
		localFiles: new(protoregistry.Files),
	}, nil
}

// Runtime returns the [goja.Runtime] this module is bound to.
func (m *Module) Runtime() *goja.Runtime {
	return m.runtime
}

// FindDescriptor looks up a descriptor by its fully-qualified name,
// searching the module's local registries first then the configured
// global registry. This is useful for resolving service, message, or
// enum descriptors by name from Go code that works alongside the JS
// module.
func (m *Module) FindDescriptor(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	return m.fileResolver().FindDescriptorByName(name)
}

// WrapMessage wraps a [dynamicpb.Message] as a JavaScript object using
// the module's standard wrapper. The returned object has the same
// shape as objects created by messageType constructors: get, set, has,
// clear, whichOneof, clearOneof methods and a $type accessor.
func (m *Module) WrapMessage(msg *dynamicpb.Message) *goja.Object {
	return m.wrapMessage(msg)
}

// UnwrapMessage extracts a [dynamicpb.Message] from a JavaScript value
// that was created by this module's [WrapMessage] or a messageType
// constructor. Returns an error if the value is not a valid protobuf
// message wrapper.
func (m *Module) UnwrapMessage(val goja.Value) (*dynamicpb.Message, error) {
	return m.unwrapMessage(val)
}

// SetupExports wires the module's JS API onto the given exports object.
// This is equivalent to the setup performed by [Enable] but allows
// external consumers to configure exports without the require() mechanism.
func (m *Module) SetupExports(exports *goja.Object) {
	m.setupExports(exports)
}

// LoadDescriptorSetBytes parses a serialized
// [google.golang.org/protobuf/types/descriptorpb.FileDescriptorSet]
// and registers all contained types into the module's local registries.
// Returns the list of registered fully-qualified type names.
func (m *Module) LoadDescriptorSetBytes(data []byte) ([]string, error) {
	return m.loadDescriptorSetBytes(data)
}

// FileResolver returns a [protodesc.Resolver] that checks the module's
// local file registries first, then falls back to the configured global
// registries. This is useful for integrating with services that need
// descriptor resolution (e.g., gRPC reflection).
func (m *Module) FileResolver() interface {
	FindFileByPath(string) (protoreflect.FileDescriptor, error)
	FindDescriptorByName(protoreflect.FullName) (protoreflect.Descriptor, error)
} {
	return m.fileResolver()
}

// TypeResolver returns a resolver that checks the module's local type
// registries first, then falls back to the configured global registries.
// The returned resolver satisfies the Resolver interface required by
// [protojson.MarshalOptions] and [protojson.UnmarshalOptions] for
// expanding google.protobuf.Any messages.
func (m *Module) TypeResolver() interface {
	FindMessageByName(protoreflect.FullName) (protoreflect.MessageType, error)
	FindMessageByURL(string) (protoreflect.MessageType, error)
	FindExtensionByName(protoreflect.FullName) (protoreflect.ExtensionType, error)
	FindExtensionByNumber(protoreflect.FullName, protoreflect.FieldNumber) (protoreflect.ExtensionType, error)
} {
	return m.typeResolver()
}

// setupExports wires the module's JS API onto the given exports object.
func (m *Module) setupExports(exports *goja.Object) {
	_ = exports.Set("loadDescriptorSet", m.jsLoadDescriptorSet)
	_ = exports.Set("loadFileDescriptorProto", m.jsLoadFileDescriptorProto)
	_ = exports.Set("messageType", m.jsMessageType)
	_ = exports.Set("enumType", m.jsEnumType)
	_ = exports.Set("encode", m.jsEncode)
	_ = exports.Set("decode", m.jsDecode)
	_ = exports.Set("toJSON", m.jsToJSON)
	_ = exports.Set("fromJSON", m.jsFromJSON)
	_ = exports.Set("equals", m.jsEquals)
	_ = exports.Set("clone", m.jsClone)
	_ = exports.Set("isMessage", m.jsIsMessage)
	_ = exports.Set("isFieldSet", m.jsIsFieldSet)
	_ = exports.Set("clearField", m.jsClearField)
	_ = exports.Set("timestampNow", m.jsTimestampNow)
	_ = exports.Set("timestampFromDate", m.jsTimestampFromDate)
	_ = exports.Set("timestampDate", m.jsTimestampDate)
	_ = exports.Set("timestampFromMs", m.jsTimestampFromMs)
	_ = exports.Set("timestampMs", m.jsTimestampMs)
	_ = exports.Set("durationFromMs", m.jsDurationFromMs)
	_ = exports.Set("durationMs", m.jsDurationMs)
	_ = exports.Set("anyPack", m.jsAnyPack)
	_ = exports.Set("anyUnpack", m.jsAnyUnpack)
	_ = exports.Set("anyIs", m.jsAnyIs)
}
