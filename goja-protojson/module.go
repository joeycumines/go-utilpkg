package gojaprotojson

import (
	"fmt"

	"github.com/dop251/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// Module provides Protocol Buffers JSON encoding/decoding for a
// [goja.Runtime]. Each Module instance is bound to a single runtime
// and uses a [gojaprotobuf.Module] for message wrapping and type
// resolution.
type Module struct {
	runtime  *goja.Runtime
	protobuf *gojaprotobuf.Module
}

// New creates a new [Module] bound to the given [goja.Runtime].
//
// New panics if runtime is nil, as this is a programming error
// (invariant violation). It returns an error if option validation
// fails.
func New(runtime *goja.Runtime, opts ...Option) (*Module, error) {
	if runtime == nil {
		panic("gojaprotojson: runtime must not be nil")
	}

	cfg, err := resolveOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("gojaprotojson: %w", err)
	}

	if cfg.protobuf == nil {
		return nil, fmt.Errorf("gojaprotojson: protobuf module is required (use WithProtobuf)")
	}

	return &Module{
		runtime:  runtime,
		protobuf: cfg.protobuf,
	}, nil
}

// SetupExports wires the module's JS API onto the given exports object.
// This is equivalent to the setup performed by [Require] but allows
// external consumers to configure exports without the require() mechanism.
func (m *Module) SetupExports(exports *goja.Object) {
	_ = exports.Set("marshal", m.runtime.ToValue(m.jsMarshal))
	_ = exports.Set("unmarshal", m.runtime.ToValue(m.jsUnmarshal))
	_ = exports.Set("format", m.runtime.ToValue(m.jsFormat))
}

// Require returns a [github.com/dop251/goja_nodejs/require.ModuleLoader]
// that registers the protojson module. This follows the standard Goja
// Node.js module pattern.
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("protojson", gojaprotojson.Require(
//		gojaprotojson.WithProtobuf(pbModule),
//	))
func Require(opts ...Option) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		m, err := New(runtime, opts...)
		if err != nil {
			panic(err)
		}
		exports := module.Get("exports").(*goja.Object)
		m.SetupExports(exports)
	}
}
