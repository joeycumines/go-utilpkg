# ADR-003: Panic vs Error Contracts

## Context

When designing APIs, a clear distinction must be made between invariants that should panic versus runtime conditions that should return errors.

## Decision

- **Panic** when the invariant is a simple contract around the API call, clearly documented, and represents a programming error that should be fixed by the caller. Examples include:
    - Nil parameters where non-nil is required.
    - Invalid configuration options that violate documented constraints.
    - Violations of preconditions that are static properties of the API contract.
- **Error** when the condition depends on external system state or may change in ways not apparent from the calling code.

## Rationale

The distinction between panics and errors allows for simplified (omitted) explicit error handling _without_ omitting it where it counts. This leads to clearer APIs and more robust code:

- **Simple contracts**: Invariants like "nil parameter is invalid" or "positive duration required" are programming errors. These violations are static properties of the API contract. Failing fast with a panic provides an immediate stack trace pointing to the misuse, which is more actionable than error propagation that obscures the source.

- **External state**: Conditions that depend on network responses, file system state, user input, or other dynamic system properties are runtime conditions. These should return errors, as they are expected situations that calling code must handle gracefully.

- **Documentation**: When panics are used for contract violations, they must be explicitly documented in the public API documentation, making the contract transparent to consumers.

## Implementation Examples

```go
package example

// Panic - static contract violation (document this clearly).
func NewClient(loop *eventloop.Loop, opts ...Option) *Client {
	if loop == nil {
		panic("loop parameter is required")
	}
	client := &Client{loop: loop}
	for _, opt := range opts {
		opt.apply(client)
	}
	return client
}

// Error - dependent on external state
func Dial(addr string) (*Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Conn{conn: conn}, nil
}

```

## Guidance

When deciding between panic and error, ask:

1. Is this a violation of a documented API contract? → _Consider_ panic
2. Does this condition depend on external factors outside the caller's control? → Error
3. Is the violation of the contract obvious from reading the call? → Only panic if yes
