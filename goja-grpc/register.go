package gojagrpc

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/goja_nodejs/require"
)

// Require returns a [require.ModuleLoader] that initialises the gRPC
// module when loaded by a [goja.Runtime]. The integrator registers the
// loader under whatever module name they choose:
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("grpc", gojagrpc.Require(
//	    gojagrpc.WithChannel(channel),
//	    gojagrpc.WithProtobuf(pbModule),
//	    gojagrpc.WithAdapter(adapter),
//	))
//	registry.Enable(runtime)
//
// After registration, JavaScript code loads the module by name:
//
//	const grpc = require('grpc');
//
// The provided options are captured and applied each time a new
// runtime calls require for this module.
func Require(opts ...ModuleOption) require.ModuleLoader {
	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Errorf("gojagrpc: %w", err))
	}
	captured := *cfg
	return func(runtime *goja.Runtime, module *goja.Object) {
		if err := authenticateRuntimeObject(runtime, module); err != nil {
			panic(runtime.NewGoError(fmt.Errorf(
				"gojagrpc: module runtime mismatch: %w",
				err,
			)))
		}
		var currentExports goja.Value
		if exception := runtime.Try(func() {
			currentExports = module.Get("exports")
		}); exception != nil {
			panic(exception)
		}
		if exports, ok := currentExports.(*goja.Object); !ok || exports == nil {
			panic(runtime.NewTypeError(
				"gojagrpc: module.exports must be an object",
			))
		}
		m, err := constructRequiredModule(runtime, &captured)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		exports := runtime.NewObject()
		if err := m.SetupExports(exports); err != nil {
			panic(runtime.NewGoError(errors.Join(err, m.Close())))
		}
		if err := m.publishExports(
			module,
			exports,
			currentExports,
		); err != nil {
			panic(runtime.NewGoError(errors.Join(
				err,
				m.Close(),
			)))
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
