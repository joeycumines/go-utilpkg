# ADR-007: Options-Accepting Constructors Return Errors

## Context

Constructors that accept a variadic options slice are forward-compatible: a new
option may add validation that the caller cannot satisfy by construction. ADR-002
instructed factories to panic on option errors. That works when every failure is a
static programming error, but option sets grow, and an option added later can
introduce a failure the caller did not anticipate.

## Decision

Constructors and factories that accept options must return an `error` rather than
panic. Option validation failures are returned to the caller.

This applies to any option-based or config-based constructor whose option set is
not closed. A nil option and any validation error inside an option's `apply`
method are returned as errors.

This supersedes the "panic on option errors" guidance in ADR-002 for
options-accepting constructors. ADR-003's distinction between static contract
violations (panic) and external-state conditions (error) still applies to
non-option constructors and to preconditions that are not option validation.

## Rationale

- An options slice is an extensibility seam; its failure mode must not harden
  into a panic just because the original option set was infallible.
- Returning the error lets the caller decide; panicking forces every future
  option to inherit a fatal contract.
- Nil options and validation errors are caller-correctable, not invariant
  violations of the constructor itself.
