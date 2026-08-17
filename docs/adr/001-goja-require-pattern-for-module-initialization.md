# ADR-001: [Goja] Require Pattern for Module Initialization

## Context

When integrating Go modules with the Goja JavaScript runtime, a consistent pattern is needed for module initialization that aligns with JavaScript ecosystem conventions while providing idiomatic Go APIs for direct usage.

## Decision

For Goja-integrated modules, provide two initialization paths:

1. **A `require.ModuleLoader` implementation** - The signature `func(runtime *goja.Runtime, module *goja.Object)` is defined by the `goja_nodejs/require` package. It is the callback invoked by the Node.js-compatible module registry; it is not, by itself, a requirement to expose a package-level function with that exact declaration.

   A module represented by an exported Go type should typically implement the callback as a bound method, for example `func (module *Module) Require(runtime *goja.Runtime, target *goja.Object)`. Binding the loader to the module instance keeps its dependencies and configuration instance-scoped. Register the method value, such as `registry.RegisterNativeModule("example", module.Require)`.

   A package-level `Require(options ...Option) require.ModuleLoader` factory remains appropriate when each runtime needs a newly constructed module and configuration must be captured before the runtime is available.

2. **An error-returning `New` constructor** - Provide a constructor for direct Go usage when module construction can fail. Its inputs follow the module's ownership model: a preconfigured module may use `New(options...)`, while a module bound to one runtime may use `New(runtime, options...)`.

## Rationale

The dual-API approach serves two distinct use cases:

- The registry requires a callback matching `require.ModuleLoader`; both a bound method value and a factory-returned closure satisfy that contract.
- JavaScript consumers expect `require()` to either succeed or throw. A loader cannot return an error, so initialization failures are raised by panicking with an appropriate Goja value or error.
- Go consumers benefit from the idiomatic error-return pattern, allowing flexible initialization and proper error propagation.
- A method receiver makes ownership explicit when a configured module instance installs its exports into one or more runtimes.

The `Require` loader exists specifically for the Goja context where the module registration callback cannot return an error. The exported API may expose that loader as a method or return it from a package-level factory. When the loader constructs the module, it should delegate validation and construction to the same error-returning path used by Go callers rather than duplicate initialization logic.

## Implementation Example

```go
type Module struct {
	// Instance-scoped dependencies and configuration.
}

// Require implements goja_nodejs/require.ModuleLoader for a configured Module.
func (module *Module) Require(runtime *goja.Runtime, target *goja.Object) {
	exports := target.Get("exports").(*goja.Object)
	module.setupExports(runtime, exports)
}

// New supports direct Go construction and normal error handling.
func New(opts ...Option) (*Module, error)

func Register(registry *require.Registry, options ...Option) error {
	configured, err := New(options...)
	if err != nil {
		return err
	}
	registry.RegisterNativeModule("example", configured.Require)
	return nil
}
```

If construction depends on the runtime, expose a loader factory instead:

```go
func Require(options ...Option) require.ModuleLoader {
	return func(runtime *goja.Runtime, target *goja.Object) {
		module, err := New(runtime, options...)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		module.Require(runtime, target)
	}
}
```
