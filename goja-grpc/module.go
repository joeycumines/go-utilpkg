package gojagrpc

import (
	"github.com/dop251/goja"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// Module provides gRPC client and server support for a [goja.Runtime].
// Each Module instance is bound to a single runtime and uses an
// [inprocgrpc.Channel] for RPC communication, a [gojaprotobuf.Module]
// for message encoding/decoding, and a [gojaeventloop.Adapter] for
// promise-based asynchronous operations.
type Module struct {
	runtime  *goja.Runtime
	channel  *inprocgrpc.Channel
	protobuf *gojaprotobuf.Module
	adapter  *gojaeventloop.Adapter
}

// New creates a new [Module] bound to the given [goja.Runtime].
//
// New panics if runtime is nil, as this is a programming error
// (invariant violation). It returns an error if option validation
// fails or if required options are missing.
//
// All three dependencies must be provided via options:
//   - [WithChannel] — the in-process gRPC channel
//   - [WithProtobuf] — the protobuf module for encode/decode
//   - [WithAdapter] — the event loop adapter for promises
func New(runtime *goja.Runtime, opts ...Option) (*Module, error) {
	if runtime == nil {
		panic("gojagrpc: runtime must not be nil")
	}

	cfg, err := resolveOptions(opts)
	if err != nil {
		return nil, err
	}

	return &Module{
		runtime:  runtime,
		channel:  cfg.channel,
		protobuf: cfg.protobuf,
		adapter:  cfg.adapter,
	}, nil
}

// Runtime returns the [goja.Runtime] this module is bound to.
func (m *Module) Runtime() *goja.Runtime {
	return m.runtime
}

// SetupExports wires the module's JS API onto the given exports object.
// This is equivalent to the setup performed by [Enable] but allows
// external consumers to configure exports without the require() mechanism.
func (m *Module) SetupExports(exports *goja.Object) {
	m.setupExports(exports)
}

// setupExports wires the module's JS API onto the given exports object.
//
// Exports:
//   - createClient — creates a gRPC client proxy for a service
//   - createServer — creates a gRPC server builder
//   - status — object with gRPC status codes and error factory
//   - metadata — object with metadata creation utilities
func (m *Module) setupExports(exports *goja.Object) {
	_ = exports.Set("createClient", m.runtime.ToValue(m.jsCreateClient))
	_ = exports.Set("createServer", m.runtime.ToValue(m.jsCreateServer))
	_ = exports.Set("createReflectionClient", m.runtime.ToValue(m.jsCreateReflectionClient))
	_ = exports.Set("enableReflection", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		m.EnableReflection()
		return goja.Undefined()
	}))
	_ = exports.Set("dial", m.runtime.ToValue(m.jsDial))
	_ = exports.Set("status", m.statusObject())
	_ = exports.Set("metadata", m.metadataObject())
}
