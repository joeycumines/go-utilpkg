# ADR-004: Prepositions Banned in Public API Names

## Context

Public API naming consistency across the project is essential for maintainability and developer experience.

## Decision

Avoid prepositions in public API names. Prefer direct, action-oriented names.

## Rationale

Preposition-free names are more idiomatic Go, shorter, clearer, and consistent with the standard library. The Go standard library consistently avoids prepositions in API names:

- `io.Reader.Read` instead of `io.Reader.ReadFrom`
- `http.Serve` instead of `http.ServeFor`
- `json.Marshal` instead of `json.MarshalToJSON`

When prepositions are eliminated, the intent is often clearer and the API surface is more concise.

## Examples

| Avoid | Prefer |
|-------|--------|
| `WaitForMessage` | `Recv` |
| `IsClosed` | `Closed` |
| `SetToValue` | `SetValue` |
| `CloseWriter` | `CloseWrite` |
| `RunInBackground` | `Run` (context makes clear) |

## Exception Cases

Prepositions may be used when:
1. The preposition is integral to the operation's meaning (e.g., `CloneFrom`)
2. Alternative names would be ambiguous or misleading
3. The operation is explicitly relative to another entity (e.g., `AddTo`)

These exceptions should be rare and carefully considered.
