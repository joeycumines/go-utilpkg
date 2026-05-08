package gojaprotobuf

import (
	"fmt"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/goja_nodejs/require"
)

// Require returns a [require.ModuleLoader] that initialises the protobuf
// module when loaded by a [goja.Runtime]. The integrator registers the
// loader under whatever module name they choose:
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
//	registry.Enable(runtime)
//
// After registration, JavaScript code loads the module by name:
//
//	const pb = require('protobuf');
//
// The provided options and default registry identities are resolved once when
// Require is called. Registry membership is snapshotted when a runtime first
// loads the returned module.
func Require(opts ...ModuleOption) require.ModuleLoader {
	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Errorf("gojaprotobuf: %w", err))
	}
	captured := *cfg
	return func(runtime *goja.Runtime, module *goja.Object) {
		if err := authenticateRuntimeObject(runtime, module); err != nil {
			panic(runtime.NewGoError(fmt.Errorf(
				"gojaprotobuf: module runtime mismatch: %w",
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
				"gojaprotobuf: module.exports must be an object",
			))
		}
		m, err := constructRequiredModule(runtime, &captured)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		if err := m.setupExports(exports); err != nil {
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
