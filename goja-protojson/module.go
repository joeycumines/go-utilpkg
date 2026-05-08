package gojaprotojson

import (
	"fmt"

	"github.com/joeycumines/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

// Module provides Protocol Buffers JSON encoding/decoding for a
// [goja.Runtime]. Each Module instance is bound to a single runtime
// and uses a [gojaprotobuf.Module] for message wrapping and type
// resolution.
type Module struct {
	runtime  *goja.Runtime
	protobuf *gojaprotobuf.Module
	state    *runtimeState
}

// New creates a new [Module] bound to the given [goja.Runtime].
//
// New panics if runtime is nil, an option is nil or invalid, the required
// protobuf option is absent, or the protobuf module belongs to another
// runtime. These are static construction contract violations. New returns an
// error when live Goja state prevents construction.
func New(runtime *goja.Runtime, opts ...ModuleOption) (*Module, error) {
	if runtime == nil {
		panic("gojaprotojson: runtime must not be nil")
	}

	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Errorf("gojaprotojson: %w", err))
	}
	return newModule(runtime, cfg)
}

func newModule(
	runtime *goja.Runtime,
	cfg *moduleConfig,
) (*Module, error) {
	if !cfg.protobuf.OwnsRuntime(runtime) {
		panic(fmt.Errorf(
			"gojaprotojson: protobuf module belongs to another runtime",
		))
	}

	state, err := acquireRuntimeState(runtime, cfg.protobuf)
	if err != nil {
		return nil, fmt.Errorf("gojaprotojson: %w", err)
	}
	return &Module{
		runtime:  runtime,
		protobuf: state.protobuf,
		state:    state,
	}, nil
}

// SetupExports wires the module's JS API onto the given exports object.
// This is equivalent to the setup performed by [Require] but allows
// external consumers to configure exports without the require() mechanism.
func (m *Module) SetupExports(exports *goja.Object) error {
	return m.installExports(exports)
}

// Require returns a [github.com/joeycumines/goja_nodejs/require.ModuleLoader]
// that registers the protojson module. This follows the standard Goja
// Node.js module pattern.
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("protojson", gojaprotojson.Require(
//		gojaprotojson.WithProtobuf(pbModule),
//	))
func Require(opts ...ModuleOption) func(runtime *goja.Runtime, module *goja.Object) {
	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Errorf("gojaprotojson: %w", err))
	}
	captured := *cfg
	return func(runtime *goja.Runtime, module *goja.Object) {
		if err := authenticateRuntimeObject(runtime, module); err != nil {
			panic(runtime.NewGoError(fmt.Errorf(
				"gojaprotojson: module runtime mismatch: %w",
				err,
			)))
		}
		var exportsValue goja.Value
		if exception := runtime.Try(func() {
			exportsValue = module.Get("exports")
		}); exception != nil {
			panic(exception)
		}
		exports, ok := exportsValue.(*goja.Object)
		if !ok || exports == nil {
			panic(runtime.NewTypeError(
				"gojaprotojson: module.exports must be an object",
			))
		}
		m, err := constructRequiredModule(runtime, &captured)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if err := m.SetupExports(exports); err != nil {
			panic(runtime.NewGoError(err))
		}
	}
}

func constructRequiredModule(
	runtime *goja.Runtime,
	cfg *moduleConfig,
) (module *Module, err error) {
	defer func() {
		reason := recover()
		if reason == nil {
			return
		}
		if _, ok := reason.(goja.Value); ok {
			panic(reason)
		}
		if constructionErr, ok := reason.(error); ok {
			panic(runtime.NewGoError(constructionErr))
		}
		panic(reason)
	}()
	return newModule(runtime, cfg)
}
