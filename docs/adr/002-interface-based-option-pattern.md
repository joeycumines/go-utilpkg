# ADR-002: Interface-Based Option Pattern

## Context

Configurable components within a package need an extensible initialization pattern that allows options to be reused across multiple implementations with overlapping configuration needs.

## Decision

Use interface-based options where option functions return concrete types that implement one or more option interfaces. When factory functions (`New`, `NewChannel`, etc.) encounter option errors, they panic.

## Rationale

The interface-based option pattern provides significant flexibility within a single package:

- **Cross-implementation reuse**: When multiple implementations exist within the same package (e.g., two distinct handlers or servers), a single option function can return a concrete type that implements multiple option interfaces. This allows the same option to be usable across all implementations without requiring duplicate option sets.

- **Ungrouped top-level functions**: Instead of grouping options by target (e.g., `HandlerOption()`, `ServerOption()`), options can be defined as standalone functions. The concrete type they return determines which interfaces they satisfy, making the API more flexible and the code organization more natural.

- **Clear error semantics**: Configuration errors are always programming errors — invalid options represent a contract violation at setup time. Panicking in factory functions on option errors provides immediate, clear feedback at the call site with a full stack trace, rather than propagating errors that must be handled by every caller.

## Implementation Example

```go
package example

// Multiple option interfaces
type HandlerOption interface {
	applyToHandler(*handlerConfig) error
}

type ServerOption interface {
	applyToServer(*serverConfig) error
}

// Single option returning concrete type satisfying both interfaces.
// Concrete type preferred if future Option interface variants are anticipated.
func WithTimeout(d time.Duration) TimeoutOption {
	return timeoutOption{d}
}

// TimeoutOption implements both [HandlerOption] and [ServerOption].
type TimeoutOption struct {
	duration time.Duration
}

func (o TimeoutOption) applyToHandler(c *handlerConfig) error {
	c.timeout = o.duration
	return nil
}

func (o TimeoutOption) applyToServer(c *serverConfig) error {
	c.timeout = o.duration
	return nil
}

// Both implementations can accept the same option
func NewHandler(opts ...HandlerOption) (*Handler, error) {
	// ...apply options, panic on errors
}

func NewServer(opts ...ServerOption) (*Server, error) {
	// ...apply options, panic on errors
}
```

## Constraint

This pattern is effective only within a single package where all option interfaces and implementations share the same package namespace.
