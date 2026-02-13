package gojagrpc

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
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
func Require(opts ...Option) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		m, err := New(runtime, opts...)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		exports := module.Get("exports").(*goja.Object)
		m.setupExports(exports)
	}
}
