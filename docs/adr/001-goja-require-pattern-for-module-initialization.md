# ADR-001: [Goja] Require Pattern for Module Initialization

## Context

When integrating Go modules with the Goja JavaScript runtime, a consistent pattern is needed for module initialization that aligns with JavaScript ecosystem conventions while providing idiomatic Go APIs for direct usage.

## Decision

For Goja-integrated modules, provide two initialization APIs:

1. **`Require(runtime *goja.Runtime, module *goja.Object)`** - Export this function for Node.js-compatible `require()` pattern. This function panics on initialization failures.

2. **`New(runtime, ...options) (*Module, error)`** - Provide this constructor for direct Go usage, returning a tuple for error handling.

## Rationale

The dual-API approach serves two distinct use cases:

- JavaScript consumers expect `require()` to either succeed or throw an uncatchable error. Panicking in `Require()` matches this contract.
- Go consumers benefit from the idiomatic error-return pattern, allowing flexible initialization and proper error propagation.

The `Require()` function exists specifically for the Goja context where error returns are not available in the module registration pattern. When using `Require()`, initialization failures are treated as programming errors (incorrect setup) rather than runtime conditions.

## Implementation Example

```go
// For Goja's require() - panics on error
func Require(_ *goja.Runtime, module *goja.Object)

// For direct Go usage - returns error
func New(opts ...Option) (*Module, error)
```
